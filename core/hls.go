// Package core provides HLS (HTTP Live Streaming) download functionality.
package core

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// hlsSegment represents a single HLS segment.
type hlsSegment struct {
	URL      string
	Index    int
	Duration float64
	// Key, when non-nil, marks an AES-128 encrypted segment.
	Key *hlsKey
}

// hlsKey describes an EXT-X-KEY encryption entry.
type hlsKey struct {
	Method string // only AES-128 is decrypted; NONE clears encryption
	URI    string // absolute key URI
	IV     []byte // 16-byte IV, or nil to derive from the media sequence
}

// hlsPlaylist represents a parsed HLS media playlist.
type hlsPlaylist struct {
	Segments []hlsSegment
	// InitSegment is the EXT-X-MAP initialization segment for fMP4 streams.
	// It must be written before the media segments or the result is not
	// decodable.
	InitSegment string
	// InitKey is the encryption applied to the init segment, if any.
	InitKey *hlsKey
	// MediaSequence is the EXT-X-MEDIA-SEQUENCE value, used to derive
	// per-segment IVs when the key tag does not carry an explicit IV.
	MediaSequence int
}

// hlsDownloader handles concurrent HLS stream downloads.
type hlsDownloader struct {
	client *http.Client
}

func newHLSDownloader() *hlsDownloader {
	return &hlsDownloader{
		// No global timeout: individual segment fetches use per-request timeouts.
		client: &http.Client{},
	}
}

// DownloadHLS downloads an HLS stream to outputPath, reporting segment progress
// via progressCallback(downloadedSegments, totalSegments). Pass nil to skip
// progress reporting.
func DownloadHLS(ctx context.Context, streamURL, outputPath, referer string, progressCallback func(int, int)) error {
	d := newHLSDownloader()
	headers := map[string]string{}
	if referer != "" {
		headers["Referer"] = referer
	}
	return d.downloadWithProgress(ctx, streamURL, outputPath, headers, progressCallback)
}

// downloadWithProgress drives the full HLS download pipeline.
//
// Reliability guarantees:
//   - Each segment is retried up to maxSegmentRetries times with exponential
//     backoff + jitter before being counted as a failure.
//   - The write loop flushes every successfully downloaded segment in order;
//     a failed segment advances nextIndex so the stream is not stalled.
//   - The download is only aborted if more than maxFailedSegments segments
//     fail permanently.
func (d *hlsDownloader) downloadWithProgress(ctx context.Context, url, output string, headers map[string]string, progressCallback func(int, int)) error {
	playlist, err := d.parsePlaylist(ctx, url, headers)
	if err != nil {
		return fmt.Errorf("failed to parse playlist: %w", err)
	}
	if len(playlist.Segments) == 0 {
		return fmt.Errorf("playlist has no segments to download")
	}

	if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	outFile, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// fMP4 HLS: write the initialization segment before any media segments,
	// otherwise the concatenated output is not decodable.
	if playlist.InitSegment != "" {
		initData, initErr := d.downloadSegment(ctx, playlist.InitSegment, headers)
		if initErr != nil {
			return fmt.Errorf("failed to download init segment: %w", initErr)
		}
		if playlist.InitKey != nil {
			iv := playlist.InitKey.IV
			if iv == nil {
				iv = sequenceIV(playlist.MediaSequence, 0)
			}
			keyBytes, keyErr := d.fetchKey(ctx, playlist.InitKey.URI, headers)
			if keyErr != nil {
				return fmt.Errorf("failed to fetch decryption key: %w", keyErr)
			}
			initData, initErr = decryptHLSSegment(initData, keyBytes, iv)
			if initErr != nil {
				return fmt.Errorf("failed to decrypt init segment: %w", initErr)
			}
		}
		if _, wErr := outFile.Write(initData); wErr != nil {
			return fmt.Errorf("failed to write init segment: %w", wErr)
		}
	}

	// Fetch decryption keys once per playlist before the workers start.
	keyBytes := map[string][]byte{}
	for _, seg := range playlist.Segments {
		if seg.Key == nil {
			continue
		}
		if _, ok := keyBytes[seg.Key.URI]; ok {
			continue
		}
		data, keyErr := d.fetchKey(ctx, seg.Key.URI, headers)
		if keyErr != nil {
			return fmt.Errorf("failed to fetch decryption key: %w", keyErr)
		}
		keyBytes[seg.Key.URI] = data
	}

	totalSegments := len(playlist.Segments)
	var downloadedSegments int32

	if progressCallback != nil {
		progressCallback(0, totalSegments)
	}

	const maxWorkers = 8
	// Allow up to 2 % of segments to fail permanently before giving up.
	// Always allow at least 3 failures for very short playlists.
	maxFailedSegments := totalSegments * 2 / 100
	if maxFailedSegments < 3 {
		maxFailedSegments = 3
	}

	type job struct {
		index   int
		segment hlsSegment
	}
	type result struct {
		index int
		data  []byte
		err   error
	}

	jobs := make(chan job, totalSegments)
	results := make(chan result, totalSegments)

	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					results <- result{index: j.index, err: ctx.Err()}
					return
				default:
				}
				data, err := d.downloadSegment(ctx, j.segment.URL, headers)
				if err == nil && j.segment.Key != nil {
					iv := j.segment.Key.IV
					if iv == nil {
						iv = sequenceIV(playlist.MediaSequence, j.index)
					}
					data, err = decryptHLSSegment(data, keyBytes[j.segment.Key.URI], iv)
				}
				results <- result{index: j.index, data: data, err: err}
			}
		}()
	}

	for i, seg := range playlist.Segments {
		jobs <- job{index: i, segment: seg}
	}
	close(jobs)

	// Buffer out-of-order results and write sequentially.
	// On segment failure we log and advance past the failed index so the
	// output file is not permanently stalled.
	segmentBuffer := make(map[int][]byte)
	nextIndex := 0
	var failedSegments int
	var firstErr error

	for i := 0; i < totalSegments; i++ {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case res := <-results:
			if res.err != nil {
				failedSegments++
				if firstErr == nil {
					firstErr = res.err
				}
				// Mark the slot as failed (nil data) so the flush loop can skip it.
				segmentBuffer[res.index] = nil
			} else {
				segmentBuffer[res.index] = res.data
				atomic.AddInt32(&downloadedSegments, 1)
			}

			// Flush all contiguous segments (including failed ones) from nextIndex.
			for {
				data, ok := segmentBuffer[nextIndex]
				if !ok {
					break
				}
				if data != nil {
					if _, wErr := outFile.Write(data); wErr != nil && firstErr == nil {
						firstErr = fmt.Errorf("failed to write segment %d: %w", nextIndex, wErr)
					}
				}
				delete(segmentBuffer, nextIndex)
				nextIndex++
			}

			if progressCallback != nil {
				progressCallback(int(atomic.LoadInt32(&downloadedSegments)), totalSegments)
			}

			if failedSegments > maxFailedSegments {
				wg.Wait()
				return fmt.Errorf("too many segment failures (%d/%d): %w", failedSegments, totalSegments, firstErr)
			}
		}
	}

	wg.Wait()
	if failedSegments > 0 {
		fmt.Printf("\n[download] Warning: %d/%d segments failed and were skipped\n", failedSegments, totalSegments)
	}
	return firstErr
}

// parsePlaylist fetches and parses a master or media playlist, returning the
// resolved media playlist for the highest-bandwidth variant.
func (d *hlsDownloader) parsePlaylist(ctx context.Context, url string, headers map[string]string) (*hlsPlaylist, error) {
	lines, err := d.fetchLines(ctx, url, headers)
	if err != nil {
		return nil, err
	}

	// Detect master playlist by presence of #EXT-X-STREAM-INF.
	isMaster := false
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			isMaster = true
			break
		}
	}

	if isMaster {
		bestURL := selectBestVariant(lines, url)
		if bestURL == "" {
			return nil, fmt.Errorf("no suitable stream found in master playlist")
		}
		return d.parseMediaPlaylist(ctx, bestURL, headers)
	}

	return parseMediaPlaylistLines(lines, url), nil
}

// parseMediaPlaylist fetches and parses a media (non-master) playlist.
func (d *hlsDownloader) parseMediaPlaylist(ctx context.Context, url string, headers map[string]string) (*hlsPlaylist, error) {
	lines, err := d.fetchLines(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	return parseMediaPlaylistLines(lines, url), nil
}

// fetchLines fetches a URL and returns its body as trimmed lines.
func (d *hlsDownloader) fetchLines(ctx context.Context, url string, headers map[string]string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var lines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}
	return lines, scanner.Err()
}

// selectBestVariant picks the highest-bandwidth variant URL from a master
// playlist's lines.
func selectBestVariant(lines []string, baseURL string) string {
	type streamInfo struct {
		URL       string
		Bandwidth int
	}

	bwRe := regexp.MustCompile(`BANDWIDTH=(\d+)`)
	var streams []streamInfo

	for i, line := range lines {
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			continue
		}
		bandwidth := 0
		if m := bwRe.FindStringSubmatch(line); len(m) > 1 {
			if bw, err := strconv.Atoi(m[1]); err == nil {
				bandwidth = bw
			}
		}
		if i+1 >= len(lines) {
			continue
		}
		variantURL := strings.TrimSpace(lines[i+1])
		if !strings.HasPrefix(variantURL, "http") {
			if idx := strings.LastIndex(baseURL, "/"); idx != -1 {
				variantURL = baseURL[:idx+1] + variantURL
			}
		}
		streams = append(streams, streamInfo{URL: variantURL, Bandwidth: bandwidth})
	}

	if len(streams) == 0 {
		return ""
	}
	best := streams[0]
	for _, s := range streams[1:] {
		if s.Bandwidth > best.Bandwidth {
			best = s
		}
	}
	return best.URL
}

// parseMediaPlaylistLines extracts segments from a media playlist's lines,
// resolving relative URLs against baseURL.
func parseMediaPlaylistLines(lines []string, baseURL string) *hlsPlaylist {
	playlist := &hlsPlaylist{}
	idx := 0
	var currentKey *hlsKey

	for i, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"))); err == nil {
				playlist.MediaSequence = n
			}
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-KEY:") {
			currentKey = parseHLSKey(line, baseURL)
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-MAP:") {
			if uri := extractAttribute(line, "URI"); uri != "" {
				playlist.InitSegment = resolvePlaylistURL(baseURL, uri)
				playlist.InitKey = currentKey
			}
			continue
		}
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		infLine := strings.TrimPrefix(line, "#EXTINF:")
		parts := strings.SplitN(infLine, ",", 2)
		var duration float64
		if len(parts) > 0 {
			duration, _ = strconv.ParseFloat(strings.TrimRight(parts[0], ", "), 64)
		}

		// The segment URI is the next non-tag line: playlists may interleave
		// tags such as #EXT-X-BITRATE between EXTINF and the URI.
		segURL := ""
		for j := i + 1; j < len(lines); j++ {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" || strings.HasPrefix(candidate, "#") {
				continue
			}
			segURL = candidate
			break
		}
		if segURL == "" {
			continue
		}
		segURL = resolvePlaylistURL(baseURL, segURL)

		playlist.Segments = append(playlist.Segments, hlsSegment{
			URL:      segURL,
			Index:    idx,
			Duration: duration,
			Key:      currentKey,
		})
		idx++
	}

	return playlist
}

// parseHLSKey parses an EXT-X-KEY tag. METHOD=NONE clears encryption; AES-128
// keys are resolved against the playlist URL.
func parseHLSKey(line, baseURL string) *hlsKey {
	method := extractRawAttribute(line, "METHOD")
	if method == "" || strings.EqualFold(method, "NONE") {
		return nil
	}
	if !strings.EqualFold(method, "AES-128") {
		// Unsupported methods (e.g. SAMPLE-AES) are left as-is; segments will
		// be downloaded raw rather than corrupted by a wrong decryption.
		return nil
	}
	key := &hlsKey{Method: "AES-128"}
	if uri := extractAttribute(line, "URI"); uri != "" {
		key.URI = resolvePlaylistURL(baseURL, uri)
	}
	if iv := extractRawAttribute(line, "IV"); iv != "" {
		key.IV = parseHLSIV(iv)
	}
	if key.URI == "" {
		return nil
	}
	return key
}

// parseHLSIV decodes a 0x-prefixed hex IV into 16 bytes.
func parseHLSIV(raw string) []byte {
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "0x"), "0X")
	if len(raw)%2 != 0 {
		raw = "0" + raw
	}
	iv := make([]byte, len(raw)/2)
	for i := 0; i < len(iv); i++ {
		value, err := strconv.ParseUint(raw[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil
		}
		iv[i] = byte(value)
	}
	if len(iv) != 16 {
		return nil
	}
	return iv
}

// resolvePlaylistURL resolves a possibly-relative playlist reference against
// the playlist URL. Root-relative references (e.g. /storage/enc.key) resolve
// against the playlist's origin.
func resolvePlaylistURL(baseURL, ref string) string {
	if strings.HasPrefix(ref, "http") {
		return ref
	}
	if strings.HasPrefix(ref, "/") {
		if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
			return u.Scheme + "://" + u.Host + ref
		}
	}
	base := baseURL
	if j := strings.LastIndex(base, "/"); j != -1 {
		base = base[:j+1]
	} else {
		base += "/"
	}
	return base + ref
}

// extractAttribute returns the value of a quoted attribute (e.g. URI="x").
func extractAttribute(line, name string) string {
	needle := name + `="`
	start := strings.Index(line, needle)
	if start == -1 {
		return ""
	}
	rest := line[start+len(needle):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// extractRawAttribute returns the value of an unquoted attribute (e.g.
// METHOD=AES-128 or IV=0x...), ending at the next comma.
func extractRawAttribute(line, name string) string {
	needle := name + "="
	start := strings.Index(line, needle)
	if start == -1 {
		return ""
	}
	rest := line[start+len(needle):]
	if end := strings.Index(rest, ","); end != -1 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest)
}

// downloadSegment fetches a single .ts segment with exponential backoff retries.
// It retries up to maxRetries times on any transient error (network timeout,
// 5xx status, read error) before giving up.
func (d *hlsDownloader) downloadSegment(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	const maxRetries = 6
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s, 16s, 32s — capped at 30s,
			// with ±25% jitter to avoid thundering herd.
			base := time.Duration(1<<uint(attempt-1)) * time.Second
			if base > 30*time.Second {
				base = 30 * time.Second
			}
			jitter := time.Duration(rand.Int63n(int64(base) / 4))
			sleep := base + jitter
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleep):
			}
		}

		// Per-request timeout: generous enough for large segments on slow CDNs.
		reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		data, err := d.fetchSegment(reqCtx, url, headers)
		cancel()

		if err == nil {
			return data, nil
		}
		lastErr = err

		// Don't retry on context cancellation from the parent.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return nil, fmt.Errorf("segment failed after %d attempts: %w", maxRetries+1, lastErr)
}

// fetchKey downloads an AES-128 key file.
func (d *hlsDownloader) fetchKey(ctx context.Context, keyURI string, headers map[string]string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := d.fetchSegment(reqCtx, keyURI, headers)
	if err != nil {
		return nil, err
	}
	if len(data) != 16 {
		return nil, fmt.Errorf("unexpected key length %d", len(data))
	}
	return data, nil
}

// sequenceIV derives the AES-128 IV from the media sequence and segment index.
func sequenceIV(mediaSequence, index int) []byte {
	iv := make([]byte, 16)
	binary.BigEndian.PutUint64(iv[8:], uint64(mediaSequence+index))
	return iv
}

// decryptHLSSegment decrypts one AES-128-CBC segment and strips PKCS7 padding.
func decryptHLSSegment(data, key, iv []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("invalid AES-128 key length %d", len(key))
	}
	if len(iv) != 16 {
		return nil, fmt.Errorf("invalid IV length %d", len(iv))
	}
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("encrypted segment is not block-aligned")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	return pkcs7Unpad(out), nil
}

// DownloadHLSAudio downloads the primary audio rendition of an HLS master
// playlist into outputPath. It returns false (with no error) when the master
// has no separate audio rendition, in which case audio is expected to be
// muxed into the video segments.
func DownloadHLSAudio(ctx context.Context, masterURL, outputPath, referer string) (bool, error) {
	d := newHLSDownloader()
	headers := map[string]string{}
	if referer != "" {
		headers["Referer"] = referer
	}

	lines, err := d.fetchLines(ctx, masterURL, headers)
	if err != nil {
		return false, err
	}
	audioURL := selectAudioRendition(lines, masterURL)
	if audioURL == "" {
		return false, nil
	}
	if err := d.downloadWithProgress(ctx, audioURL, outputPath, headers, nil); err != nil {
		return false, err
	}
	return true, nil
}

// selectAudioRendition picks an audio rendition URI from master playlist lines,
// preferring the DEFAULT=YES entry and returning "" when none exists.
func selectAudioRendition(lines []string, masterURL string) string {
	var fallback string
	for _, line := range lines {
		if !strings.HasPrefix(line, "#EXT-X-MEDIA:") {
			continue
		}
		if !strings.EqualFold(extractRawAttribute(line, "TYPE"), "AUDIO") {
			continue
		}
		uri := extractAttribute(line, "URI")
		if uri == "" {
			continue
		}
		resolved := resolvePlaylistURL(masterURL, uri)
		if strings.EqualFold(extractRawAttribute(line, "DEFAULT"), "YES") {
			return resolved
		}
		if fallback == "" {
			fallback = resolved
		}
	}
	return fallback
}

// fetchSegment performs a single HTTP GET for a segment URL.
func (d *hlsDownloader) fetchSegment(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}

	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if readErr != nil {
		return nil, readErr
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return body, nil
}
