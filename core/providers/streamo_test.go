package providers

import (
	"net/http"
	"testing"
)

func TestStreamoParseID(t *testing.T) {
	movie, err := parseStreamoID("movie|1153576")
	if err != nil {
		t.Fatal(err)
	}
	if movie.kind != "movie" || movie.tmdb != "1153576" {
		t.Fatalf("unexpected movie parse: %+v", movie)
	}
	if got := streamoID(movie); got != "movie|1153576" {
		t.Fatalf("movie roundtrip mismatch: %s", got)
	}

	show, err := parseStreamoID("series|1396")
	if err != nil {
		t.Fatal(err)
	}
	if show.kind != "series" || show.tmdb != "1396" || show.season != "" || show.episode != "" {
		t.Fatalf("unexpected show parse: %+v", show)
	}
	if got := streamoID(show); got != "series|1396" {
		t.Fatalf("show roundtrip mismatch: %s", got)
	}

	season, err := parseStreamoID("series|1396|1")
	if err != nil {
		t.Fatal(err)
	}
	if season.season != "1" || season.episode != "" {
		t.Fatalf("unexpected season parse: %+v", season)
	}
	if got := streamoID(season); got != "series|1396|1" {
		t.Fatalf("season roundtrip mismatch: %s", got)
	}

	ep, err := parseStreamoID("series|1396|1|1")
	if err != nil {
		t.Fatal(err)
	}
	if ep.kind != "series" || ep.tmdb != "1396" || ep.season != "1" || ep.episode != "1" {
		t.Fatalf("unexpected episode parse: %+v", ep)
	}
	if got := streamoID(ep); got != "series|1396|1|1" {
		t.Fatalf("episode roundtrip mismatch: %s", got)
	}

	for _, bad := range []string{"", "movie", "movie|", "tv|123", "episode|1|2|3", "|1396"} {
		if _, err := parseStreamoID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestStreamoGetMediaID(t *testing.T) {
	s := NewStreamo(&http.Client{})

	id, err := s.GetMediaID("https://streamo.st/movies/1153576")
	if err != nil || id != "movie|1153576" {
		t.Fatalf("bare movie URL: %q %v", id, err)
	}

	// Slug suffixes used by Streamo's public routes must be stripped.
	id, err = s.GetMediaID("https://streamo.st/movies/1153576-street-fighter-2026")
	if err != nil || id != "movie|1153576" {
		t.Fatalf("slug movie URL: %q %v", id, err)
	}

	id, err = s.GetMediaID("https://streamo.st/series/1396-breaking-bad-2008")
	if err != nil || id != "series|1396" {
		t.Fatalf("slug series URL: %q %v", id, err)
	}

	for _, bad := range []string{
		"https://streamo.st/",
		"https://streamo.st/movies",
		"https://streamo.st/contact",
		"https://streamo.st/movies/",
		"://bad url",
	} {
		if id, err := s.GetMediaID(bad); err == nil {
			t.Errorf("URL %q produced ID %q", bad, id)
		}
	}
}

func TestStreamoGetLinkRejectsIncompleteSeries(t *testing.T) {
	s := NewStreamo(&http.Client{})
	// Show-level and season-level IDs have no episode: resolution must refuse
	// before touching the network.
	if _, err := s.GetLink("series|1396"); err == nil {
		t.Fatal("show-level ID accepted")
	}
	if _, err := s.GetLink("series|1396|1"); err == nil {
		t.Fatal("season-level ID accepted")
	}
	if _, err := s.GetLink("series|1396|abc|1"); err == nil {
		t.Fatal("non-numeric season accepted")
	}
}
