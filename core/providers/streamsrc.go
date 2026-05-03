package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/demonkingswarn/luffy/core"
)

type StreamSrc struct {
	Client  *http.Client
	tmdbID  string
	embedURL string
}

func NewStreamSrc(client *http.Client) *StreamSrc {
	return &StreamSrc{Client: client}
}

func (s *StreamSrc) Search(query string) ([]core.SearchResult, error) {
	tmdbID, err := core.GetTMDBMovieID(query, s.Client)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://streamsrc.cc/watch/movie/%d", tmdbID)

	return []core.SearchResult{
		{
			Title: query,
			URL:   url,
			Type:  core.Movie,
		},
	}, nil
}

func (s *StreamSrc) GetMediaID(url string) (string, error) {
	re := regexp.MustCompile(`/watch/movie/(\d+)`)
	m := re.FindStringSubmatch(url)
	if len(m) < 2 {
		return "", fmt.Errorf("invalid streamsrc url")
	}
	s.tmdbID = m[1]
	return m[1], nil
}

func (s *StreamSrc) GetSeasons(mediaID string) ([]core.Season, error) {
	return nil, nil
}

func (s *StreamSrc) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	apiURL := fmt.Sprintf("https://streamsrc.cc/watch/movie/%s?json=1", id)

	resp, err := s.Client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var nested map[string]map[string][]map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&nested); err == nil {
		for _, providerData := range nested {
			movies, ok := providerData["Movie"]
			if !ok {
				continue
			}
			for _, movie := range movies {
				if embedURL, ok := movie["embed_url"]; ok && embedURL != "" {
					s.embedURL = embedURL
					return []core.Episode{
						{
							ID:   embedURL,
							Name: movie["episode_title"],
						},
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no embed URL found or movie not in database")
}

func (s *StreamSrc) GetServers(episodeID string) ([]core.Server, error) {
	return nil, nil
}

func (s *StreamSrc) GetLink(serverID string) (string, error) {
	if s.embedURL != "" {
		return s.embedURL, nil
	}
	return serverID, nil
}