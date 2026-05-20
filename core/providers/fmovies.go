package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/demonkingswarn/luffy/core"
)

const (
	FMOVIES_BASE_URL = "https://www.fmovies.gd"
)

type Fmovies struct {
	Client *http.Client
}

func NewFmovies(client *http.Client) *Fmovies {
	return &Fmovies{Client: client}
}

func (f *Fmovies) newTMDBRequest(path string, params url.Values) (*http.Request, error) {
	params.Set("api_key", core.TMDB_API_KEY)
	fullURL := fmt.Sprintf("%s/%s?%s", core.TMDB_BASE_URL, path, params.Encode())
	req, err := core.NewRequest("GET", fullURL)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (f *Fmovies) Search(query string) ([]core.SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("include_adult", "false")
	params.Set("language", "en-US")
	params.Set("page", "1")

	req, err := f.newTMDBRequest("search/multi", params)
	if err != nil {
		return nil, err
	}

	resp, err := f.Client.Do(req)
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
			URL:    fmt.Sprintf("%s/%s/%d?title=%s&year=%s", FMOVIES_BASE_URL, r.MediaType, r.ID, url.QueryEscape(title), year),
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

func (f *Fmovies) GetMediaID(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid fmovies URL")
	}

	mediaType := parts[0]
	if mediaType == "tv" {
		mediaType = "series"
	}

	return strings.Join([]string{parts[1], mediaType}, "|"), nil
}

func (f *Fmovies) GetSeasons(mediaID string) ([]core.Season, error) {
	parts := strings.Split(mediaID, "|")
	if len(parts) < 2 || parts[1] != "series" {
		return nil, nil
	}

	req, err := f.newTMDBRequest(fmt.Sprintf("tv/%s", parts[0]), url.Values{})
	if err != nil {
		return nil, err
	}

	resp, err := f.Client.Do(req)
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
			ID:   fmt.Sprintf("%s|%d", parts[0], s.SeasonNumber),
			Name: s.Name,
		})
	}
	return seasons, nil
}

func (f *Fmovies) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	parts := strings.Split(id, "|")
	if !isSeason {
		return []core.Episode{{ID: fmt.Sprintf("%s|0|0", parts[0]), Name: "Movie"}}, nil
	}
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid fmovies season ID")
	}

	req, err := f.newTMDBRequest(fmt.Sprintf("tv/%s/season/%s", parts[0], parts[1]), url.Values{})
	if err != nil {
		return nil, err
	}

	resp, err := f.Client.Do(req)
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
			ID:   fmt.Sprintf("%s|%s|%d", parts[0], parts[1], e.EpisodeNumber),
			Name: fmt.Sprintf("E%02d - %s", e.EpisodeNumber, e.Name),
		})
	}
	return episodes, nil
}

func (f *Fmovies) GetServers(episodeID string) ([]core.Server, error) {
	parts := strings.Split(episodeID, "|")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid fmovies episode ID")
	}

	tmdbID := parts[0]
	season, _ := strconv.Atoi(parts[1])
	episode, _ := strconv.Atoi(parts[2])

	var embedURL string
	if season == 0 && episode == 0 {
		embedURL = fmt.Sprintf("%s/embed/movie/%s", VIDKING_BASE_URL, tmdbID)
	} else {
		embedURL = fmt.Sprintf("%s/embed/tv/%s/%d/%d", VIDKING_BASE_URL, tmdbID, season, episode)
	}

	return []core.Server{{ID: embedURL, Name: "Fmovies"}}, nil
}

func (f *Fmovies) GetLink(serverID string) (string, error) {
	return resolveVidKingEmbed(serverID, f.Client)
}
