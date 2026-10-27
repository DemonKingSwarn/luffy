package providers

import (
	"os"
	"strings"
	"testing"

	"github.com/demonkingswarn/luffy/core"
)

// Live API check; run with STREAMO_LIVE=1.
func TestStreamoGetLinkLive(t *testing.T) {
	if os.Getenv("STREAMO_LIVE") == "" {
		t.Skip("set STREAMO_LIVE=1 to run")
	}
	s := NewStreamo(core.NewClient())

	for _, tc := range []struct{ name, id string }{
		{"movie", "movie|1153576"},
		{"series", "series|1396|1|1"},
	} {
		stream, err := s.GetLink(tc.id)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		t.Logf("%s -> %.160s", tc.name, stream)
		if !strings.Contains(stream, "streamo.st") && !strings.Contains(stream, ".m3u8") {
			t.Fatalf("%s: unexpected stream URL: %s", tc.name, stream)
		}
	}
}
