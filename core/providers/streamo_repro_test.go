package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/demonkingswarn/luffy/core"
)

// Reproduces the user-reported series failure (VisionQuest, TMDB 213375):
// GetLink succeeds, history saves, but mpv never renders a frame.
// Guarded by STREAMO_REPRO=1. Add STREAMO_MPV=1 to actually launch mpv
// with the exact argument list buildPlayerCmd produces.
func TestStreamoSeriesRepro(t *testing.T) {
	if os.Getenv("STREAMO_REPRO") != "1" {
		t.Skip("set STREAMO_REPRO=1 to run")
	}

	s := NewStreamo(core.NewClient())

	for _, ep := range []string{"1", "4"} {
		ep := ep
		t.Run("s1e"+ep, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			_ = ctx

			start := time.Now()
			stream, err := s.GetLink("series|213375|1|" + ep)
			if err != nil {
				t.Fatalf("GetLink failed: %v", err)
			}
			t.Logf("GetLink took %s -> %s", time.Since(start).Round(time.Millisecond), stream)

			probeStream(t, stream)

			if os.Getenv("STREAMO_MPV") == "1" {
				launchMpvLikeLuffy(t, stream, "VisionQuest - E0"+ep)
			}
		})
	}
}

// probeStream fetches the playlist and its first referenced segments with the
// same headers mpv receives (UA/Referer/Origin).
func probeStream(t *testing.T, streamURL string) {
	base := streamURL
	body := fetchLikeMpv(t, base)
	if !strings.HasPrefix(strings.TrimLeft(body, " \t\r\n"), "#EXTM3U") {
		t.Fatalf("playlist is not HLS (first 200 bytes): %.200q", body)
	}
	t.Logf("playlist OK (%d bytes)", len(body))

	// Collect URIs (for a master, probe through the first variant).
	var uris []string
	isMaster := strings.Contains(body, "#EXT-X-STREAM-INF")
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		uris = append(uris, line)
	}
	if isMaster {
		if len(uris) == 0 {
			t.Fatal("master playlist has no variants")
		}
		t.Logf("master playlist with %d variants; probing first variant", len(uris))
		base = resolveRef(base, uris[0])
		vbody := fetchLikeMpv(t, base)
		if !strings.HasPrefix(strings.TrimLeft(vbody, " \t\r\n"), "#EXTM3U") {
			t.Fatalf("variant is not HLS: %.200q", vbody)
		}
		body = vbody
		uris = nil
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			uris = append(uris, line)
		}
	}

	limit := len(uris)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		segURL := resolveRef(base, uris[i])
		req, _ := http.NewRequest(http.MethodGet, segURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Origin", StreamoBaseURL)
		req.Header.Set("Referer", StreamoBaseURL+"/")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("segment %d fetch error: %v", i, err)
			return
		}
		n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		ct := resp.Header.Get("Content-Type")
		t.Logf("segment %d: HTTP %d, %d bytes, %s, %s", i, resp.StatusCode, n, ct, trunc(segURL, 90))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Errorf("segment %d returned HTTP %d", i, resp.StatusCode)
		}
	}
}

func fetchLikeMpv(t *testing.T, rawURL string) string {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("bad URL %q: %v", rawURL, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Origin", StreamoBaseURL)
	req.Header.Set("Referer", StreamoBaseURL+"/")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch %s: %v", trunc(rawURL, 90), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("fetch %s: HTTP %d", trunc(rawURL, 90), resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatalf("read %s: %v", trunc(rawURL, 90), err)
	}
	return string(body)
}

// launchMpvLikeLuffy runs mpv with the exact argument list the Linux mpv
// branch of buildPlayerCmd produces (user's real mpv.conf applies).
// --term-playing-msg is the only addition: it prints PLAYING_OK the moment
// real playback starts, distinguishing "streaming fine" from "stuck loading".
func launchMpvLikeLuffy(t *testing.T, streamURL, title string) {
	sock := fmt.Sprintf("/tmp/mpv-ipc-repro-%d.sock", os.Getpid())
	args := []string{
		streamURL,
		"--referrer=" + StreamoBaseURL + "/",
		"--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"--force-media-title=Playing " + title,
		"--http-header-fields=Origin: " + StreamoBaseURL,
		"--input-ipc-server=" + sock,
		"--term-playing-msg=PLAYING_OK time=${time-pos}",
	}
	// Cap the run: a healthy stream keeps mpv alive for the full episode.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "mpv", args...)
	out, err := cmd.CombinedOutput()
	played := strings.Contains(string(out), "PLAYING_OK")
	t.Logf("mpv exit err=%v (timeoutHit=%v) playbackStarted=%v\noutput tail:\n%s",
		err, ctx.Err() != nil, played, tail(string(out), 1500))
	if !played {
		t.Errorf("mpv never reached playback state")
	}
}

func resolveRef(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
