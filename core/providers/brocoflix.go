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
	BROCOFLIX_BASE_URL = "https://brocoflix.xyz"
)

type Brocoflix struct {
	Client *http.Client
}

func NewBrocoflix(client *http.Client) *Brocoflix {
	return &Brocoflix{Client: client}
}

func (b *Brocoflix) Search(query string) ([]core.SearchResult, error) {
	u, _ := url.Parse(core.TMDB_BASE_URL + "/search/multi")
	q := u.Query()
	q.Set("api_key", core.TMDB_API_KEY)
	q.Set("query", query)
	u.RawQuery = q.Encode()

	resp, err := b.Client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []core.SearchResult
	for _, item := range data.Results {
		if item.MediaType != "movie" && item.MediaType != "tv" {
			continue
		}

		title := item.Title
		if item.MediaType == "tv" {
			title = item.Name
		}

		poster := ""
		if item.PosterPath != "" {
			poster = "https://image.tmdb.org/t/p/w500" + item.PosterPath
		}

		mediaType := core.Movie
		if item.MediaType == "tv" {
			mediaType = core.Series
		}

		resURL := fmt.Sprintf("%s/pages/info.html?id=%d&type=%s", BROCOFLIX_BASE_URL, item.ID, item.MediaType)

		results = append(results, core.SearchResult{
			Title:  title,
			URL:    resURL,
			Type:   mediaType,
			Poster: poster,
		})
	}

	return results, nil
}

func (b *Brocoflix) GetMediaID(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	q := u.Query()
	id := q.Get("id")
	mediaType := q.Get("type")

	if id == "" || mediaType == "" {
		return "", fmt.Errorf("invalid url")
	}

	return fmt.Sprintf("%s:%s", mediaType, id), nil
}

func (b *Brocoflix) GetSeasons(mediaID string) ([]core.Season, error) {
	parts := strings.Split(mediaID, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid media id format")
	}
	mediaType := parts[0]
	id := parts[1]

	if mediaType == "movie" {
		return nil, nil
	}

	u := fmt.Sprintf("%s/tv/%s?api_key=%s", core.TMDB_BASE_URL, id, core.TMDB_API_KEY)
	resp, err := b.Client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var details core.TmdbShowDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}

	var seasons []core.Season
	for _, s := range details.Seasons {
		if s.SeasonNumber == 0 {
			continue
		}

		sid := fmt.Sprintf("series:%s:%d", id, s.SeasonNumber)
		seasons = append(seasons, core.Season{
			ID:   sid,
			Name: s.Name,
		})
	}
	return seasons, nil
}

func (b *Brocoflix) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if !isSeason {

		servers := []core.Server{
			{Name: "VidSrc", ID: "vidsrc:" + id},
			{Name: "MultiEmbed", ID: "multiembed:" + id},
			{Name: "VidLink", ID: "vidlink:" + id},
			{Name: "EmbedSu", ID: "embedsu:" + id},
		}

		var eps []core.Episode
		for _, s := range servers {
			eps = append(eps, core.Episode{
				ID:   s.ID,
				Name: s.Name,
			})
		}
		return eps, nil
	}

	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid season id")
	}
	showID := parts[1]
	seasonNum := parts[2]

	u := fmt.Sprintf("%s/tv/%s/season/%s?api_key=%s", core.TMDB_BASE_URL, showID, seasonNum, core.TMDB_API_KEY)
	resp, err := b.Client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSeasonDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var episodes []core.Episode
	for _, ep := range data.Episodes {
		eid := fmt.Sprintf("%s:%d", id, ep.EpisodeNumber)
		episodes = append(episodes, core.Episode{
			ID:   eid,
			Name: fmt.Sprintf("Episode %d: %s", ep.EpisodeNumber, ep.Name),
		})
	}

	return episodes, nil
}

func (b *Brocoflix) GetServers(episodeID string) ([]core.Server, error) {
	servers := []core.Server{
		{Name: "VidSrc", ID: "vidsrc:" + episodeID},
		{Name: "MultiEmbed", ID: "multiembed:" + episodeID},
		{Name: "VidLink", ID: "vidlink:" + episodeID},
		{Name: "EmbedSu", ID: "embedsu:" + episodeID},
	}
	return servers, nil
}

func (b *Brocoflix) GetLink(serverID string) (string, error) {
	parts := strings.Split(serverID, ":")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid server id")
	}

	serverName := parts[0]
	mediaType := parts[1]
	tmdbID := parts[2]

	season := ""
	episode := ""
	if mediaType == "series" && len(parts) >= 5 {
		season = parts[3]
		episode = parts[4]
	}

	var embedLink string

	switch serverName {
	case "vidsrc":
		if mediaType == "movie" {
			embedLink = fmt.Sprintf("https://vidsrc.xyz/embed/movie/%s", tmdbID)
		} else {
			embedLink = fmt.Sprintf("https://vidsrc.xyz/embed/tv/%s/%s/%s", tmdbID, season, episode)
		}

	case "multiembed":
		if mediaType == "movie" {
			embedLink = fmt.Sprintf("https://multiembed.mov/?video_id=%s&tmdb=1", tmdbID)
		} else {
			embedLink = fmt.Sprintf("https://multiembed.mov/?video_id=%s&tmdb=1&s=%s&e=%s", tmdbID, season, episode)
		}

	case "vidlink":
		if mediaType == "movie" {
			embedLink = fmt.Sprintf("https://vidlink.pro/movie/%s", tmdbID)
		} else {
			embedLink = fmt.Sprintf("https://vidlink.pro/tv/%s/%s/%s", tmdbID, season, episode)
		}

	case "embedsu":
		if mediaType == "movie" {
			embedLink = fmt.Sprintf("https://embed.su/embed/movie/%s", tmdbID)
		} else {
			embedLink = fmt.Sprintf("https://embed.su/embed/tv/%s/%s/%s", tmdbID, season, episode)
		}
	default:
		return "", fmt.Errorf("unknown server: %s", serverName)
	}

	return embedLink, nil
}
