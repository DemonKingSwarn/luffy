package cmd

import (
	"testing"

	"github.com/demonkingswarn/luffy/core"
	"github.com/demonkingswarn/luffy/core/providers"
)

// Streamo's GetLink already returns a resolved HLS URL; passing it through
// DecryptStream used to fail with "decryption failed". The URL must flow
// through untouched with the site referer attached.
func TestResolveStreamURLStreamoPassthrough(t *testing.T) {
	ctx := &core.Context{Client: core.NewClient()}
	cfg := &core.Config{}

	link := "https://streamo.st/api/streaming/proxy?ref=abc123"
	stream, referer, subtitles, err := resolveStreamURL(link, ctx, cfg, "streamo", "Test Movie", false, false)
	if err != nil {
		t.Fatalf("resolveStreamURL failed: %v", err)
	}
	if stream != link {
		t.Fatalf("stream URL altered: %q", stream)
	}
	if referer != providers.StreamoBaseURL+"/" {
		t.Fatalf("referer = %q, want site root", referer)
	}
	if len(subtitles) != 0 {
		t.Fatalf("unexpected subtitles: %v", subtitles)
	}
}
