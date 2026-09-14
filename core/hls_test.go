package core

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"strings"
	"testing"
)

func TestParseMediaPlaylistInitSegment(t *testing.T) {
	// fMP4 HLS playlists declare an init segment via EXT-X-MAP.
	lines := []string{
		"#EXTM3U",
		"#EXT-X-VERSION:7",
		"#EXT-X-TARGETDURATION:10",
		`#EXT-X-MAP:URI="init.mp4"`,
		"#EXTINF:10.0,",
		"playlist_000.m4s",
		"#EXTINF:10.0,",
		"playlist_001.m4s",
	}
	playlist := parseMediaPlaylistLines(lines, "https://cdn.example/hls/master/video_1080p.m3u8")
	if playlist.InitSegment != "https://cdn.example/hls/master/init.mp4" {
		t.Fatalf("unexpected init segment: %q", playlist.InitSegment)
	}
	if len(playlist.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(playlist.Segments))
	}
	if playlist.Segments[0].URL != "https://cdn.example/hls/master/playlist_000.m4s" {
		t.Fatalf("unexpected first segment: %q", playlist.Segments[0].URL)
	}

	// Absolute init URLs are preserved.
	abs := parseMediaPlaylistLines([]string{
		`#EXT-X-MAP:URI="https://other.example/init.mp4"`,
		"#EXTINF:6.0,",
		"a.ts",
	}, "https://cdn.example/x.m3u8")
	if abs.InitSegment != "https://other.example/init.mp4" {
		t.Fatalf("absolute init not preserved: %q", abs.InitSegment)
	}

	// Playlists without EXT-X-MAP keep an empty init segment.
	none := parseMediaPlaylistLines([]string{"#EXTINF:6.0,", "a.ts"}, "https://cdn.example/x.m3u8")
	if none.InitSegment != "" {
		t.Fatalf("unexpected init segment: %q", none.InitSegment)
	}
}

func TestParseMediaPlaylistEncryption(t *testing.T) {
	lines := []string{
		"#EXTM3U",
		"#EXT-X-MEDIA-SEQUENCE:5",
		`#EXT-X-KEY:METHOD=AES-128,URI="/storage/enc.key",IV=0x43A6D967D5C17290D98322F5C8F6660B`,
		"#EXTINF:8.0,",
		"0000.ts",
		"#EXTINF:8.0,",
		"0001.ts",
	}
	playlist := parseMediaPlaylistLines(lines, "https://vixsrc.to/playlist/620626?type=video")
	if playlist.MediaSequence != 5 {
		t.Fatalf("media sequence not parsed: %d", playlist.MediaSequence)
	}
	if len(playlist.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(playlist.Segments))
	}
	key := playlist.Segments[0].Key
	if key == nil {
		t.Fatal("segment key missing")
	}
	// Root-relative key URIs resolve against the playlist origin.
	if key.URI != "https://vixsrc.to/storage/enc.key" {
		t.Fatalf("unexpected key URI: %q", key.URI)
	}
	if len(key.IV) != 16 || key.IV[0] != 0x43 {
		t.Fatalf("unexpected IV: %v", key.IV)
	}
	if playlist.Segments[1].Key == nil {
		t.Fatal("key should apply to subsequent segments")
	}

	// METHOD=NONE clears encryption for following segments.
	cleared := parseMediaPlaylistLines([]string{
		`#EXT-X-KEY:METHOD=AES-128,URI="k.key",IV=0x00`,
		"#EXTINF:4.0,", "a.ts",
		"#EXT-X-KEY:METHOD=NONE",
		"#EXTINF:4.0,", "b.ts",
	}, "https://cdn.example/x.m3u8")
	if cleared.Segments[0].Key == nil {
		t.Fatal("first segment should be encrypted")
	}
	if cleared.Segments[1].Key != nil {
		t.Fatal("second segment should be plaintext after METHOD=NONE")
	}
}

func TestDecryptHLSSegmentRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := []byte("abcdef0123456789")
	plain := []byte("The quick brown fox jumps over the lazy dog")

	// Encrypt with PKCS7 padding using the same scheme the downloader expects.
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)

	decrypted, err := decryptHLSSegment(encrypted, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("round trip mismatch: %q", decrypted)
	}

	if _, err := decryptHLSSegment([]byte{1, 2, 3}, key, iv); err == nil {
		t.Fatal("non-block-aligned segment accepted")
	}
	if _, err := decryptHLSSegment(encrypted, key[:8], iv); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestSequenceIV(t *testing.T) {
	iv := sequenceIV(5, 2)
	if len(iv) != 16 || iv[15] != 7 {
		t.Fatalf("unexpected derived IV: %v", iv)
	}
}

func TestParseMediaPlaylistInterleavedTags(t *testing.T) {
	// Some CDNs (Cinejoy/movieboxnoob) emit #EXT-X-BITRATE between EXTINF and
	// the segment URI. The URI must still be found.
	lines := []string{
		"#EXTM3U",
		`#EXT-X-MAP:URI="init.mp4"`,
		"#EXTINF:10.41667,",
		"#EXT-X-BITRATE:3358",
		"seg_000.html",
		"#EXTINF:10.41667,",
		"#EXT-X-BITRATE:2975",
		"seg_001.html",
		"#EXTINF:10.0,",
		"seg_002.html",
	}
	playlist := parseMediaPlaylistLines(lines, "https://cdn.example/video/v.m3u8")
	if len(playlist.Segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(playlist.Segments))
	}
	want := []string{
		"https://cdn.example/video/seg_000.html",
		"https://cdn.example/video/seg_001.html",
		"https://cdn.example/video/seg_002.html",
	}
	for i, w := range want {
		if playlist.Segments[i].URL != w {
			t.Fatalf("segment %d: got %q want %q", i, playlist.Segments[i].URL, w)
		}
	}
}

func TestSelectAudioRendition(t *testing.T) {
	lines := []string{
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="Italian",DEFAULT=YES,URI="audio_it.m3u8"`,
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="English",DEFAULT=NO,URI="audio_en.m3u8"`,
		`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",URI="subs.m3u8"`,
		`#EXT-X-STREAM-INF:BANDWIDTH=1,AUDIO="audio"`,
		"video.m3u8",
	}
	got := selectAudioRendition(lines, "https://vixsrc.to/playlist/1?x=1")
	if !strings.HasSuffix(got, "/audio_it.m3u8") {
		t.Fatalf("default audio not selected: %q", got)
	}

	// Without a DEFAULT entry the first audio rendition wins.
	noDefault := []string{`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",URI="a1.m3u8"`, `#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",URI="a2.m3u8"`}
	if got := selectAudioRendition(noDefault, "https://x/y.m3u8"); !strings.HasSuffix(got, "/a1.m3u8") {
		t.Fatalf("first audio not selected: %q", got)
	}

	// No audio renditions at all.
	if got := selectAudioRendition([]string{"#EXT-X-STREAM-INF:BANDWIDTH=1", "v.m3u8"}, "https://x/y.m3u8"); got != "" {
		t.Fatalf("expected no rendition, got %q", got)
	}
}
