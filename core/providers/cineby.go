package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/demonkingswarn/luffy/core"
)

const (
	CINEBY_BASE_URL  = "https://www.cineby.sc"
	VIDKING_BASE_URL = "https://www.vidking.net"
)

type Cineby struct {
	Client *http.Client
}

func NewCineby(client *http.Client) *Cineby {
	return &Cineby{Client: client}
}

func (c *Cineby) newTMDBRequest(path string, params url.Values) (*http.Request, error) {
	params.Set("api_key", core.TMDB_API_KEY)
	fullURL := fmt.Sprintf("%s/%s?%s", core.TMDB_BASE_URL, path, params.Encode())
	req, err := core.NewRequest("GET", fullURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", CINEBY_BASE_URL+"/")
	return req, nil
}

func (c *Cineby) Search(query string) ([]core.SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("include_adult", "false")
	params.Set("language", "en-US")
	params.Set("page", "1")

	req, err := c.newTMDBRequest("search/multi", params)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []core.SearchResult
	for _, r := range data.Results {
		if r.MediaType != "movie" && r.MediaType != "tv" {
			continue
		}

		title := r.Title
		if title == "" {
			title = r.Name
		}

		year := r.ReleaseDate
		if year == "" {
			year = r.FirstAirDate
		}
		if len(year) > 4 {
			year = year[:4]
		}

		mediaType := core.Movie
		if r.MediaType == "tv" {
			mediaType = core.Series
		}

		poster := ""
		if r.PosterPath != "" {
			poster = core.TMDB_IMAGE_BASE_URL + r.PosterPath
		}

		results = append(results, core.SearchResult{
			Title:  title,
			URL:    fmt.Sprintf("%s/%s/%d?title=%s&year=%s", CINEBY_BASE_URL, r.MediaType, r.ID, url.QueryEscape(title), year),
			Type:   mediaType,
			Poster: poster,
			Year:   year,
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no results")
	}
	return results, nil
}

func (c *Cineby) GetMediaID(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid cineby URL")
	}

	mediaType := parts[0]
	if mediaType == "tv" {
		mediaType = "series"
	}

	return strings.Join([]string{
		parts[1],
		mediaType,
		u.Query().Get("title"),
		u.Query().Get("year"),
	}, "|"), nil
}

func (c *Cineby) GetSeasons(mediaID string) ([]core.Season, error) {
	parts := strings.Split(mediaID, "|")
	if len(parts) < 2 || parts[1] != "series" {
		return nil, nil
	}

	req, err := c.newTMDBRequest(fmt.Sprintf("tv/%s", parts[0]), url.Values{})
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbShowDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var seasons []core.Season
	for _, s := range data.Seasons {
		if s.SeasonNumber == 0 {
			continue
		}
		seasons = append(seasons, core.Season{
			ID:   fmt.Sprintf("%s|%d|%s|%s", parts[0], s.SeasonNumber, safePart(parts, 2), safePart(parts, 3)),
			Name: s.Name,
		})
	}
	return seasons, nil
}

func (c *Cineby) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	parts := strings.Split(id, "|")
	if !isSeason {
		return []core.Episode{{ID: fmt.Sprintf("%s|0|0|%s|%s", parts[0], safePart(parts, 2), safePart(parts, 3)), Name: "Movie"}}, nil
	}
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid cineby season ID")
	}

	req, err := c.newTMDBRequest(fmt.Sprintf("tv/%s/season/%s", parts[0], parts[1]), url.Values{})
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSeasonDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var episodes []core.Episode
	for _, e := range data.Episodes {
		episodes = append(episodes, core.Episode{
			ID:   fmt.Sprintf("%s|%s|%d|%s|%s", parts[0], parts[1], e.EpisodeNumber, safePart(parts, 2), safePart(parts, 3)),
			Name: fmt.Sprintf("E%02d - %s", e.EpisodeNumber, e.Name),
		})
	}
	return episodes, nil
}

func (c *Cineby) GetServers(episodeID string) ([]core.Server, error) {
	parts := strings.Split(episodeID, "|")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid cineby episode ID")
	}

	serverID := fmt.Sprintf("%s/embed/movie/%s", VIDKING_BASE_URL, parts[0])
	name := "VidKing"
	if parts[1] != "0" {
		season, _ := strconv.Atoi(parts[1])
		episode, _ := strconv.Atoi(parts[2])
		serverID = fmt.Sprintf("%s/embed/tv/%s/%d/%d", VIDKING_BASE_URL, parts[0], season, episode)
		name = "VidKing TV"
	}

	return []core.Server{{ID: serverID, Name: name}}, nil
}

func (c *Cineby) GetLink(serverID string) (string, error) {
	return resolveVidKingEmbed(serverID)
}

func resolveVidKingEmbed(embedURL string) (string, error) {
	if _, err := exec.LookPath("agent-browser"); err != nil {
		return "", fmt.Errorf("agent-browser is required to resolve VidKing embeds: %w", err)
	}

	if !strings.Contains(embedURL, "autoPlay=") {
		sep := "?"
		if strings.Contains(embedURL, "?") {
			sep = "&"
		}
		embedURL += sep + "autoPlay=true"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sessionArgs := []string{"--session", "luffy-vidking"}
	if output, err := exec.CommandContext(ctx, "agent-browser", append(sessionArgs, "open", embedURL)...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to open VidKing embed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.CommandContext(ctx, "agent-browser", append(sessionArgs, "wait", "9000")...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed waiting for VidKing playback: %w: %s", err, strings.TrimSpace(string(output)))
	}

	script := `JSON.stringify(performance.getEntriesByType('resource').map(r => r.name).filter(n => /\.m3u8(\?|$)/i.test(n)))`
	output, err := exec.CommandContext(ctx, "agent-browser", append(sessionArgs, "eval", script)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to inspect VidKing resources: %w: %s", err, strings.TrimSpace(string(output)))
	}

	m3u8s := extractM3U8URLs(string(output))
	if len(m3u8s) == 0 {
		return "", fmt.Errorf("no playable m3u8 found in VidKing embed")
	}
	return m3u8s[0], nil
}

func extractM3U8URLs(text string) []string {
	re := regexp.MustCompile(`https?://[^"\\\s]+\.m3u8[^"\\\s]*`)
	matches := re.FindAllString(text, -1)
	seen := make(map[string]bool, len(matches))
	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		match = strings.ReplaceAll(match, `\/`, `/`)
		match = strings.ReplaceAll(match, `\u0026`, `&`)
		if !seen[match] {
			seen[match] = true
			urls = append(urls, match)
		}
	}
	return urls
}

func safePart(parts []string, idx int) string {
	if idx >= len(parts) {
		return ""
	}
	return parts[idx]
}
