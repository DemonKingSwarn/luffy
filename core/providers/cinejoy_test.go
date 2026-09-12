package providers

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCinejoyParseID(t *testing.T) {
	movie, err := parseCinejoyID("movie|1108427|The Goonies|1985")
	if err != nil {
		t.Fatal(err)
	}
	if movie.kind != "movie" || movie.tmdb != "1108427" || movie.title != "The Goonies" || movie.year != "1985" {
		t.Fatalf("unexpected movie parse: %+v", movie)
	}
	if got := cinejoyID(movie); got != "movie|1108427|The Goonies|1985" {
		t.Fatalf("movie roundtrip mismatch: %s", got)
	}

	ep, err := parseCinejoyID("series|1396|1|1|Breaking Bad|2008")
	if err != nil {
		t.Fatal(err)
	}
	if ep.kind != "series" || ep.tmdb != "1396" || ep.season != "1" || ep.episode != "1" || ep.title != "Breaking Bad" {
		t.Fatalf("unexpected series parse: %+v", ep)
	}
	if got := cinejoyID(ep); got != "series|1396|1|1|Breaking Bad|2008" {
		t.Fatalf("series roundtrip mismatch: %s", got)
	}

	for _, bad := range []string{"", "movie", "movie|", "tv|123|1|1|x", "episode|1|2|3"} {
		if _, err := parseCinejoyID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
	// Lenient show-level series IDs (no season/episode) parse; resolvability is
	// enforced in GetLink.
	show, err := parseCinejoyID("series|1396|Breaking Bad|2008")
	if err != nil || show.kind != "series" || show.tmdb != "1396" {
		t.Fatalf("show-level series ID failed: %+v", err)
	}
}

func TestCinejoySealedRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	eph := make([]byte, 65)
	if _, err := rand.Read(eph); err != nil {
		t.Fatal(err)
	}
	sealed := &cjSealed{responseKey: key, keyID: 7, ephemeralPublic: eph}

	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	want := cjEnvelope{Status: 200, Data: json.RawMessage(`{"stream":[]}`)}
	plain, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	encrypted := gcm.Seal(nil, iv, plain, cinejoyAAD(sealed))
	wire := append(iv, encrypted...)

	got, err := cjDecrypt(sealed, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 200 || !bytes.Equal(got.Data, want.Data) {
		t.Fatalf("decrypt mismatch: %+v", got)
	}

	// Tampered ciphertext must fail authentication.
	wire[len(wire)-1] ^= 1
	if _, err := cjDecrypt(sealed, wire); err == nil {
		t.Fatal("tampered response authenticated")
	}

	// Wrong key must fail.
	other := *sealed
	other.responseKey = append([]byte(nil), key...)
	other.responseKey[0] ^= 1
	wire[len(wire)-1] ^= 1
	if _, err := cjDecrypt(&other, wire); err == nil {
		t.Fatal("wrong key authenticated")
	}

	// Short wire must be rejected before decryption.
	if _, err := cjDecrypt(sealed, make([]byte, 12)); err == nil {
		t.Fatal("short wire accepted")
	}
}

func TestCinejoySealedHeaderLayout(t *testing.T) {
	sealed := splitCinejoySealed(make([]byte, 99))
	if len(sealed.responseKey) != 32 || len(sealed.ephemeralPublic) != 65 || len(sealed.body) != 1 {
		t.Fatalf("unexpected header layout")
	}
	// Callers guard with a length check before calling splitCinejoySealed;
	// a short input must panic rather than mis-slice.
	defer func() {
		if recover() == nil {
			t.Fatal("short sealed input did not panic")
		}
	}()
	splitCinejoySealed(make([]byte, 98))
}

func TestCinejoyExtractCandidates(t *testing.T) {
	data := json.RawMessage(`{
		"stream": [
			{"type": "file", "qualities": {"1080": {"url": "https://a.example/master.m3u8"}, "720": {"url": "https://a.example/hd.m3u8"}, "bad": {"url": ""}}},
			{"playlist": "javascript:alert(1)"},
			{"playlist": "https://b.example/play.m3u8", "captions": [{"lang": "en"}]}
		]
	}`)
	cands := cjExtractCandidates(data)
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	// File qualities sorted by key descending, empties dropped.
	if cands[0].url != "https://a.example/master.m3u8" || cands[1].url != "https://a.example/hd.m3u8" {
		t.Fatalf("quality sort failed: %+v", cands[:2])
	}
	if cands[2].url != "https://b.example/play.m3u8" || cands[2].captions != 1 {
		t.Fatalf("playlist candidate failed: %+v", cands[2])
	}

	if cands := cjExtractCandidates(json.RawMessage(`{"stream":[]}`)); len(cands) != 0 {
		t.Fatalf("empty stream list produced candidates")
	}
	if cands := cjExtractCandidates(json.RawMessage(`null`)); len(cands) != 0 {
		t.Fatalf("null data produced candidates")
	}
	if cands := cjExtractCandidates(json.RawMessage(`{"stream":[{"playlist":"http://plain.example/x.m3u8"}]}`)); len(cands) != 0 {
		t.Fatalf("http (non-https) playlist accepted")
	}
}

func TestCinejoyCopyParamsEmbed(t *testing.T) {
	base := map[string]any{"tmdb": "11"}
	params := copyParams(base, json.RawMessage(`"embed-1"`))
	if params["embed"] != "embed-1" {
		t.Fatalf("string embed not carried: %+v", params)
	}
	params = copyParams(base, json.RawMessage(`42`))
	if params["embed"] != float64(42) {
		t.Fatalf("numeric embed not carried: %+v", params)
	}
	params = copyParams(base, json.RawMessage(`null`))
	if _, ok := params["embed"]; ok {
		t.Fatalf("null embed carried: %+v", params)
	}
	if params["tmdb"] != "11" {
		t.Fatalf("base params lost: %+v", params)
	}
}

func TestCinejoyFetchURLGuards(t *testing.T) {
	client := &http.Client{}
	for _, bad := range []string{
		"ftp://cinejoy.to/x",
		"https://user:pass@cinejoy.to/x",
		"http://cinejoy.to/x",
		"https://localhost/x",
		"https://127.0.0.1/x",
		"https://internal.host/x",
		"https://x.local/x",
	} {
		if _, err := cjFetch(context.Background(), client, http.MethodGet, bad, nil, 1024); err == nil {
			t.Errorf("URL %q was accepted", bad)
		}
	}
}

func TestCinejoyIDSanitizesPipes(t *testing.T) {
	// Titles containing pipe characters must not corrupt the ID encoding.
	id := cinejoyID(cjRequest{kind: "movie", tmdb: "123", title: "A | B", year: "1999"})
	if strings.Contains(id, "A | B") {
		t.Fatalf("pipe not sanitized: %s", id)
	}
	cj, err := parseCinejoyID(id)
	if err != nil || cj.title != "A - B" {
		t.Fatalf("sanitized title not parsed back: %+v", err)
	}
}
