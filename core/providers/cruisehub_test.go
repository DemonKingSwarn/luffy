package providers

import (
	"strings"
	"testing"

	"github.com/demonkingswarn/luffy/core"
)

// TestCruisehubLive walks the full chain end-to-end against live services.
// Hits the network — skip with -short.
//
//	go test -v -run TestCruisehubLive ./core/providers/
func TestCruisehubLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}

	client := core.NewClient()
	p := NewCruisehub(client)

	t.Run("search", func(t *testing.T) {
		results, err := p.Search("dune")
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		t.Logf("got %d results", len(results))
		for i, r := range results {
			if i >= 5 {
				break
			}
			t.Logf("  [%s] %s (%s) %s", r.Type, r.Title, r.Year, r.URL)
		}
	})

	// Dune (2021) — TMDB 438631
	movieURL := "https://flix.cruisehub.live/movie/438631"
	movieMID, err := p.GetMediaID(movieURL)
	if err != nil {
		t.Fatalf("GetMediaID(movie): %v", err)
	}
	t.Logf("movie mediaID: %s", movieMID)

	// Breaking Bad — TMDB 1396
	tvURL := "https://flix.cruisehub.live/tv/1396"
	tvMID, _ := p.GetMediaID(tvURL)
	t.Logf("tv mediaID: %s", tvMID)

	t.Run("seasons-tv", func(t *testing.T) {
		seasons, err := p.GetSeasons(tvMID)
		if err != nil {
			t.Fatalf("GetSeasons: %v", err)
		}
		if len(seasons) == 0 {
			t.Fatal("no seasons")
		}
		t.Logf("seasons: %d, first: %+v", len(seasons), seasons[0])

		eps, err := p.GetEpisodes(seasons[0].ID, true)
		if err != nil {
			t.Fatalf("GetEpisodes: %v", err)
		}
		if len(eps) == 0 {
			t.Fatal("no episodes")
		}
		t.Logf("episodes in S1: %d, first: %+v", len(eps), eps[0])
	})

	t.Run("decrypt-movie", func(t *testing.T) {
		servers, _ := p.GetServers(movieMID)
		t.Logf("got %d servers", len(servers))
		for _, s := range servers {
			link, err := p.GetLink(s.ID)
			if err != nil {
				t.Logf("  %-12s GetLink: %v", s.Name, err)
				continue
			}
			t.Logf("  %-12s -> %s", s.Name, link)

			stream, subs, ref, err := core.DecryptStream(link, client)
			if err != nil {
				t.Logf("    DecryptStream: %v", err)
				continue
			}
			isM3U8 := strings.Contains(strings.ToLower(stream), ".m3u8")
			t.Logf("    OK m3u8=%v subs=%d referer=%s\n      stream=%s",
				isM3U8, len(subs), ref, stream)
		}
	})

	t.Run("decrypt-tv", func(t *testing.T) {
		seasons, _ := p.GetSeasons(tvMID)
		eps, _ := p.GetEpisodes(seasons[0].ID, true)
		servers, _ := p.GetServers(eps[0].ID)
		t.Logf("got %d servers for S1E1", len(servers))
		for _, s := range servers {
			link, err := p.GetLink(s.ID)
			if err != nil {
				t.Logf("  %-12s GetLink: %v", s.Name, err)
				continue
			}
			t.Logf("  %-12s -> %s", s.Name, link)

			stream, subs, ref, err := core.DecryptStream(link, client)
			if err != nil {
				t.Logf("    DecryptStream: %v", err)
				continue
			}
			isM3U8 := strings.Contains(strings.ToLower(stream), ".m3u8")
			t.Logf("    OK m3u8=%v subs=%d referer=%s\n      stream=%s",
				isM3U8, len(subs), ref, stream)
		}
	})
}
