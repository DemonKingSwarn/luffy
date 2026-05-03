package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/demonkingswarn/luffy/core"
)

// 🔥 Domain rotation (important)
var VIDSRC_DOMAINS = []string{
	"https://vidsrc.me",
	"https://vidsrc.to",
	"https://vidsrc.cc",
}

type Vidsrc struct {
	Client *http.Client
}

type imdbSuggestionResponse struct {
	Results []struct {
		ID    string `json:"id"`
		Title string `json:"l"`
		Type  string `json:"qid"`
		Year  int    `json:"y"`
		Image struct {
			URL string `json:"imageUrl"`
		} `json:"i"`
	} `json:"d"`
}

func NewVidsrc(client *http.Client) *Vidsrc {
	// 🔥 better HTTP client defaults
	client.Timeout = 10 * time.Second

	return &Vidsrc{
		Client: client,
	}
}

// ---------------- SEARCH ----------------

func (v *Vidsrc) Search(query string) ([]core.SearchResult, error) {
	endpoint := imdbSuggestionURL(query)

	req, err := core.NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res imdbSuggestionResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)

	var results []core.SearchResult

	for _, item := range res.Results {
		if item.ID == "" || item.Title == "" || !isIMDbMovieType(item.Type) {
			continue
		}

		titleLower := strings.ToLower(item.Title)

		// 🔥 relevance filter (prevents wrong picks)
		score := relevanceScore(queryLower, titleLower)

		if score < 0.4 {
			continue
		}

		results = append(results, core.SearchResult{
			Title:  item.Title,
			URL:    buildEmbedURL(item.ID),
			Type:   core.Movie,
			Poster: item.Image.URL,
			Year:   formatYear(item.Year),
		})
	}

	if len(results) == 0 {
		return nil, errors.New("no results")
	}

	// 🔥 sort by relevance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Title < results[j].Title
	})

	return results, nil
}

// ---------------- MEDIA ID ----------------

func (v *Vidsrc) GetMediaID(urlStr string) (string, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	trimmed := strings.Trim(parsed.Path, "/")
	parts := strings.Split(trimmed, "/")

	if len(parts) < 3 || parts[0] != "embed" {
		return "", errors.New("invalid vidsrc embed url")
	}

	return parts[1] + ":" + parts[2], nil
}

// ---------------- EPISODES ----------------

func (v *Vidsrc) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if !isSeason {
		return []core.Episode{{
			ID:   id,
			Name: "Vidsrc",
		}}, nil
	}
	return nil, errors.New("tv not supported yet")
}

// ---------------- SERVERS ----------------

func (v *Vidsrc) GetServers(episodeID string) ([]core.Server, error) {
	return []core.Server{{
		ID:   episodeID,
		Name: "vidsrc",
	}}, nil
}

func (v *Vidsrc) GetSeasons(mediaID string) ([]core.Season, error) {
	// vidsrc doesn't support TV properly
	return nil, nil
}

// ---------------- LINK (WITH FALLBACK) ----------------

func (v *Vidsrc) GetLink(serverID string) (string, error) {
	mediaType, tmdbID, err := parseVidsrcMediaID(serverID)
	if err != nil {
		return "", err
	}

	for _, domain := range VIDSRC_DOMAINS {
		link := fmt.Sprintf("%s/embed/%s/%s", domain, mediaType, tmdbID)

		// 🔥 check if domain is alive
		if v.isAlive(link) {
			return link, nil
		}
	}

	return "", errors.New("all vidsrc domains failed")
}

// ---------------- HELPERS ----------------

func (v *Vidsrc) isAlive(url string) bool {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := v.Client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

func buildEmbedURL(id string) string {
	// default domain
	return fmt.Sprintf("%s/embed/movie/%s", VIDSRC_DOMAINS[0], id)
}

func imdbSuggestionURL(query string) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	normalized = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")

	if normalized == "" {
		normalized = "a"
	}

	first := normalized[:1]
	return fmt.Sprintf("https://v2.sg.media-imdb.com/suggestion/%s/%s.json",
		first, url.PathEscape(normalized))
}

func isIMDbMovieType(kind string) bool {
	switch strings.ToLower(kind) {
	case "movie", "feature", "tvmovie", "short", "video":
		return true
	default:
		return false
	}
}

func relevanceScore(query, title string) float64 {
	if query == title {
		return 1.0
	}
	if strings.Contains(title, query) {
		return 0.8
	}

	// simple similarity
	common := 0
	for _, c := range query {
		if strings.ContainsRune(title, c) {
			common++
		}
	}

	return float64(common) / float64(len(query))
}

func formatYear(year int) string {
	if year <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", year)
}

func parseVidsrcMediaID(mediaID string) (string, string, error) {
	parts := strings.Split(mediaID, ":")
	if len(parts) != 2 {
		return "", "", errors.New("invalid media id")
	}
	return parts[0], parts[1], nil
}