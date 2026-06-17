// Package providers — Cruisehub provider.
//
// Cruisehub (flix.cruisehub.live) is a TMDB-driven SPA whose only job is to
// pick a TMDB id and hand it off to one of several third-party embed players.
// We mirror that behaviour: TMDB for search/seasons/episodes, then return the
// embed URL list for each server. The actual stream extraction is done by
// core.DecryptStream which already handles vidlink.pro and vidsrc-embed.ru.
//
// ID encoding (pipe-separated):
//
//	mediaID   = "{tmdb}|{movie|tv}"
//	seasonID  = "{tmdb}|tv|{season}"
//	episodeID = "{tmdb}|tv|{season}|{episode}"  (or mediaID for movies)
//	serverID  = "{embedID}|{rest of episodeID/mediaID}"
//
// Embed URL formats are lifted verbatim from the cruisehub bundle (xL/bL).
package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/demonkingswarn/luffy/core"
)

const (
	CRUISEHUB_BASE_URL = "https://flix.cruisehub.live"
	CRUISEHUB_REFERER  = CRUISEHUB_BASE_URL + "/"
)

// cruisehubEmbed is a single third-party embed source. Order of the slice
// below is the order servers are presented to the user (and auto-picked for
// TV episodes). We list embeds we know how to decrypt first.
type cruisehubEmbed struct {
	ID   string
	Name string
}

// cruisehubEmbeds is ordered: things core.DecryptStream already handles
// first, last-resort embeds last. vidsrc-embed handles both movie/tv via
// cloudnestra; vidlink currently only decrypts movies but is left here as
// a fallback for movie playback.
var cruisehubEmbeds = []cruisehubEmbed{
	{ID: "vidsrc-embed", Name: "Vidsrc"},
	{ID: "vidlink", Name: "Vidlink"},
	{ID: "vidvault", Name: "Vidvault"},
	{ID: "primesrc", Name: "Primesrc"},
}

type Cruisehub struct {
	Client *http.Client
}

func NewCruisehub(client *http.Client) *Cruisehub {
	return &Cruisehub{Client: client}
}

func (c *Cruisehub) Search(query string) ([]core.SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("include_adult", "false")
	params.Set("language", "en-US")
	params.Set("page", "1")
	params.Set("api_key", core.TMDB_API_KEY)

	endpoint := fmt.Sprintf("%s/search/multi?%s", core.TMDB_BASE_URL, params.Encode())
	req, err := core.NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb search: %w", err)
	}
	defer resp.Body.Close()

	var data core.TmdbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode tmdb response: %w", err)
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
		mt := core.Movie
		if r.MediaType == "tv" {
			mt = core.Series
		}
		poster := ""
		if r.PosterPath != "" {
			poster = core.TMDB_IMAGE_BASE_URL + r.PosterPath
		}
		results = append(results, core.SearchResult{
			Title:  title,
			URL:    fmt.Sprintf("%s/%s/%d", CRUISEHUB_BASE_URL, r.MediaType, r.ID),
			Type:   mt,
			Poster: poster,
			Year:   year,
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no results")
	}
	return results, nil
}

// GetMediaID parses a cruisehub URL like https://flix.cruisehub.live/movie/123
// into "{tmdb}|{type}".
func (c *Cruisehub) GetMediaID(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", fmt.Errorf("parse cruisehub url: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid cruisehub url: %s", mediaURL)
	}
	mtype := parts[0]
	if mtype != "movie" && mtype != "tv" {
		return "", fmt.Errorf("unsupported media type: %s", mtype)
	}
	return fmt.Sprintf("%s|%s", parts[1], mtype), nil
}

// GetSeasons hits TMDB /tv/{id} and returns one Season per non-zero
// season_number. For movies this returns nil — cmd/root.go falls back to
// the movie flow which calls GetServers(mediaID) directly.
func (c *Cruisehub) GetSeasons(mediaID string) ([]core.Season, error) {
	tmdb, mtype, _, _, err := splitID(mediaID)
	if err != nil {
		return nil, err
	}
	if mtype != "tv" {
		return nil, nil
	}

	endpoint := fmt.Sprintf("%s/tv/%s?api_key=%s&language=en-US",
		core.TMDB_BASE_URL, tmdb, core.TMDB_API_KEY)
	req, err := core.NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb tv details: %w", err)
	}
	defer resp.Body.Close()

	var data core.TmdbShowDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode tv details: %w", err)
	}

	var seasons []core.Season
	for _, s := range data.Seasons {
		if s.SeasonNumber == 0 {
			// Skip "Specials" — same as videasy/cineby behaviour.
			continue
		}
		seasons = append(seasons, core.Season{
			ID:   fmt.Sprintf("%s|tv|%d", tmdb, s.SeasonNumber),
			Name: s.Name,
		})
	}
	return seasons, nil
}

// GetEpisodes:
//   - isSeason=true:  id is a seasonID, returns episodes from TMDB.
//   - isSeason=false: id is a movie mediaID, returns one synthetic Movie entry.
func (c *Cruisehub) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	tmdb, mtype, season, _, err := splitID(id)
	if err != nil {
		return nil, err
	}

	if !isSeason {
		// Movie path: cmd/root.go's series branch sometimes calls GetEpisodes(mediaID, false)
		// (e.g. on history-resume when a movie was recorded). Return one entry that
		// we'll treat as the mediaID itself when GetServers is called.
		return []core.Episode{{ID: id, Name: "Movie"}}, nil
	}

	if mtype != "tv" || season == "" {
		return nil, fmt.Errorf("invalid season id: %s", id)
	}

	endpoint := fmt.Sprintf("%s/tv/%s/season/%s?api_key=%s&language=en-US",
		core.TMDB_BASE_URL, tmdb, season, core.TMDB_API_KEY)
	req, err := core.NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb season details: %w", err)
	}
	defer resp.Body.Close()

	var data core.TmdbSeasonDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode season details: %w", err)
	}

	var episodes []core.Episode
	for _, e := range data.Episodes {
		episodes = append(episodes, core.Episode{
			ID:   fmt.Sprintf("%s|tv|%s|%d", tmdb, season, e.EpisodeNumber),
			Name: fmt.Sprintf("E%02d - %s", e.EpisodeNumber, e.Name),
		})
	}
	return episodes, nil
}

// GetServers returns the embed list. The episodeID (or mediaID for movies)
// is prefixed with each embed ID so GetLink can reconstruct the URL.
func (c *Cruisehub) GetServers(episodeID string) ([]core.Server, error) {
	servers := make([]core.Server, 0, len(cruisehubEmbeds))
	for _, e := range cruisehubEmbeds {
		servers = append(servers, core.Server{
			ID:   fmt.Sprintf("%s|%s", e.ID, episodeID),
			Name: e.Name,
		})
	}
	return servers, nil
}

// GetLink builds the third-party embed URL for the given server ID.
func (c *Cruisehub) GetLink(serverID string) (string, error) {
	parts := strings.SplitN(serverID, "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid cruisehub server id: %s", serverID)
	}
	embedID, rest := parts[0], parts[1]

	tmdb, mtype, season, episode, err := splitID(rest)
	if err != nil {
		return "", err
	}
	link := buildCruisehubEmbedURL(embedID, mtype, tmdb, season, episode)
	if link == "" {
		return "", fmt.Errorf("unknown cruisehub embed: %s", embedID)
	}
	return link, nil
}

// buildCruisehubEmbedURL replicates the xL/bL functions from the cruisehub
// frontend bundle. Keep these formats in sync if the bundle ever changes.
func buildCruisehubEmbedURL(embedID, mtype, tmdb, season, episode string) string {
	tv := mtype == "tv" && season != "" && episode != ""

	switch embedID {
	case "vidsrc-embed":
		if tv {
			return fmt.Sprintf("https://vidsrc-embed.ru/embed/tv/%s/%s/%s", tmdb, season, episode)
		}
		return fmt.Sprintf("https://vidsrc-embed.ru/embed/movie/%s", tmdb)

	case "primesrc":
		if tv {
			return fmt.Sprintf("https://primesrc.me/embed/tv?tmdb=%s&season=%s&episode=%s", tmdb, season, episode)
		}
		return fmt.Sprintf("https://primesrc.me/embed/movie?tmdb=%s", tmdb)

	case "vidlink":
		if tv {
			return fmt.Sprintf("https://vidlink.pro/tv/%s/%s/%s", tmdb, season, episode)
		}
		return fmt.Sprintf("https://vidlink.pro/movie/%s", tmdb)

	case "vidvault":
		if tv {
			return fmt.Sprintf("https://vidvault.ru/tv/%s/%s/%s", tmdb, season, episode)
		}
		return fmt.Sprintf("https://vidvault.ru/movie/%s", tmdb)
	}
	return ""
}

// splitID parses pipe-separated IDs in any of the supported shapes:
//
//	"{tmdb}|{type}"                   → mediaID
//	"{tmdb}|tv|{season}"              → seasonID
//	"{tmdb}|tv|{season}|{episode}"    → episodeID
//
// Returns (tmdb, type, season, episode, err). Missing fields come back empty.
func splitID(id string) (tmdb, mtype, season, episode string, err error) {
	parts := strings.Split(id, "|")
	if len(parts) < 2 {
		return "", "", "", "", fmt.Errorf("invalid id: %s", id)
	}
	tmdb = parts[0]
	mtype = parts[1]
	if len(parts) >= 3 {
		season = parts[2]
	}
	if len(parts) >= 4 {
		episode = parts[3]
	}
	return tmdb, mtype, season, episode, nil
}
