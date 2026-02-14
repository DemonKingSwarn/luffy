package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/demonkingswarn/luffy/core"
)

const (
	FLIXHQ_BASE_URL   = "https://flixhq.to"
	FLIXHQ_SEARCH_URL = FLIXHQ_BASE_URL + "/search"
	FLIXHQ_AJAX_URL   = FLIXHQ_BASE_URL + "/ajax"
)

type FlixHQ struct {
	Client *http.Client
}

func NewFlixHQ(client *http.Client) *FlixHQ {
	return &FlixHQ{Client: client}
}

func (f *FlixHQ) newRequest(method, url string) (*http.Request, error) {
	req, err := core.NewRequest(method, url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://flixhq.to/")
	return req, nil
}

func (f *FlixHQ) Search(query string) ([]core.SearchResult, error) {
	search := strings.ReplaceAll(query, " ", "-")
	req, _ := f.newRequest("GET", FLIXHQ_SEARCH_URL+"/"+search)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []core.SearchResult

	doc.Find("div.flw-item").EachWithBreak(func(i int, s *goquery.Selection) bool {
		if i >= 10 {
			return false
		}

		title := s.Find("h2.film-name a").AttrOr("title", "Unknown")
		href := s.Find("div.film-poster a").AttrOr("href", "")
		poster := s.Find("img.film-poster-img").AttrOr("data-src", "")
		typeStr := strings.TrimSpace(s.Find("span.fdi-type").Text())

		mediaType := core.Movie
		if strings.EqualFold(typeStr, "TV") || strings.EqualFold(typeStr, "Series") {
			mediaType = core.Series
		}

		if href != "" {
			results = append(results, core.SearchResult{
				Title:  title,
				URL:    FLIXHQ_BASE_URL + href,
				Type:   mediaType,
				Poster: poster,
			})
		}
		return true
	})

	if len(results) == 0 {
		return nil, errors.New("no results")
	}

	return results, nil
}

func (f *FlixHQ) GetMediaID(url string) (string, error) {
	req, _ := f.newRequest("GET", url)
	resp, err := f.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	id := doc.Find("#watch-block").AttrOr("data-id", "")
	if id == "" {
		id = doc.Find("div.detail_page-watch").AttrOr("data-id", "")
	}
	if id == "" {
		id = doc.Find("#movie_id").AttrOr("value", "")
	}

	if id == "" {
		return "", fmt.Errorf("could not find media ID")
	}
	return id, nil
}

func (f *FlixHQ) GetSeasons(mediaID string) ([]core.Season, error) {
	url := fmt.Sprintf("%s/season/list/%s", FLIXHQ_AJAX_URL, mediaID)
	req, _ := f.newRequest("GET", url)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var seasons []core.Season
	doc.Find(".dropdown-item").Each(func(i int, s *goquery.Selection) {
		id := s.AttrOr("data-id", "")
		name := strings.TrimSpace(s.Text())
		if id != "" {
			seasons = append(seasons, core.Season{ID: id, Name: name})
		}
	})
	return seasons, nil
}

func (f *FlixHQ) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	endpoint := "movie/episodes"
	if isSeason {
		endpoint = "season/episodes"
	}
	url := fmt.Sprintf("%s/%s/%s", FLIXHQ_AJAX_URL, endpoint, id)

	req, _ := f.newRequest("GET", url)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var episodes []core.Episode
	doc.Find(".nav-item a").Each(func(i int, s *goquery.Selection) {
		id := s.AttrOr("data-id", "")
		if id == "" {
			id = s.AttrOr("data-linkid", "")
		}
		name := strings.TrimSpace(s.AttrOr("title", s.Text()))
		if name == "" {
			name = s.Text()
		}
		if id != "" {
			episodes = append(episodes, core.Episode{ID: id, Name: name})
		}
	})

	if len(episodes) == 0 {
		doc.Find("a.eps-item").Each(func(i int, s *goquery.Selection) {
			id := s.AttrOr("data-id", "")
			name := strings.TrimSpace(s.AttrOr("title", s.Text()))
			if id != "" {
				episodes = append(episodes, core.Episode{ID: id, Name: name})
			}
		})
	}
	return episodes, nil
}

func (f *FlixHQ) GetServers(episodeID string) ([]core.Server, error) {
	url := fmt.Sprintf("%s/episode/servers/%s", FLIXHQ_AJAX_URL, episodeID)
	req, _ := f.newRequest("GET", url)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var servers []core.Server
	doc.Find(".nav-item a").Each(func(i int, s *goquery.Selection) {
		id := s.AttrOr("data-id", "")
		name := strings.TrimSpace(s.Find("span").Text())
		if name == "" {
			name = strings.TrimSpace(s.Text())
		}
		if id != "" {
			servers = append(servers, core.Server{ID: id, Name: name})
		}
	})
	return servers, nil
}

func (f *FlixHQ) GetLink(serverID string) (string, error) {
	url := fmt.Sprintf("%s/episode/sources/%s", FLIXHQ_AJAX_URL, serverID)
	req, _ := f.newRequest("GET", url)
	resp, err := f.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Link, nil
}
