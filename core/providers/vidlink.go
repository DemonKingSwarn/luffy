package providers

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/demonkingswarn/luffy/core"
)

type Vidlink struct {
	Client *http.Client
	tmdbID string
}

func NewVidlink(client *http.Client) *Vidlink {
	return &Vidlink{Client: client}
}

func (v *Vidlink) Search(query string) ([]core.SearchResult, error) {
	tmdbID, err := core.GetTMDBMovieID(query, v.Client)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://vidlink.pro/movie/%d", tmdbID)

	return []core.SearchResult{
		{
			Title: query,
			URL:   url,
			Type:  core.Movie,
		},
	}, nil
}

func (v *Vidlink) GetMediaID(url string) (string, error) {
	re := regexp.MustCompile(`/movie/(\d+)`)
	m := re.FindStringSubmatch(url)
	if len(m) < 2 {
		return "", fmt.Errorf("invalid vidlink url")
	}
	v.tmdbID = m[1]
	return m[1], nil
}

func (v *Vidlink) GetSeasons(mediaID string) ([]core.Season, error) {
	return nil, nil
}

func (v *Vidlink) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	return []core.Episode{
		{
			ID:   v.tmdbID,
			Name: "Movie",
		},
	}, nil
}

func (v *Vidlink) GetServers(episodeID string) ([]core.Server, error) {
	return []core.Server{
		{
			ID:   fmt.Sprintf("https://vidlink.pro/movie/%s", episodeID),
			Name: "vidlink",
		},
	}, nil
}

func (v *Vidlink) GetLink(serverID string) (string, error) {
	if !regexp.MustCompile(`^https?://`).MatchString(serverID) {
		return fmt.Sprintf("https://vidlink.pro/movie/%s", serverID), nil
	}
	return serverID, nil
}