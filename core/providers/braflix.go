package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/demonkingswarn/luffy/core"
)

const (
	BRAFLIX_BASE_URL = "https://braflix.nl"
	BRAFLIX_AJAX_URL = "https://braflix.nl/ajax"
)

type Braflix struct {
	Client *http.Client
}

func NewBraflix(client *http.Client) *Braflix {
	return &Braflix{Client: client}
}

func (b *Braflix) newRequest(method, urlStr string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	return req, nil
}

func (b *Braflix) Search(query string) ([]core.SearchResult, error) {
	encodedQuery := url.QueryEscape(strings.ReplaceAll(query, " ", "-"))
	// Braflix uses /search/query-string
	searchURL := fmt.Sprintf("%s/search/%s", BRAFLIX_BASE_URL, encodedQuery)

	req, _ := b.newRequest("GET", searchURL, nil)
	// Remove X-Requested-With for search page
	req.Header.Del("X-Requested-With")

	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []core.SearchResult

	doc.Find("div.flw-item").Each(func(i int, s *goquery.Selection) {
		title := s.Find("h2.film-name a").AttrOr("title", "")
		href := s.Find("h2.film-name a").AttrOr("href", "")
		poster := s.Find("img.film-poster-img").AttrOr("data-src", "")
		if poster == "" {
			poster = s.Find("img.film-poster-img").AttrOr("src", "")
		}

		// Type inference
		mediaType := core.Movie
		if strings.Contains(href, "/tv/") {
			mediaType = core.Series
		}

		info := s.Find("div.film-infor").Text()
		year := ""
		// Extract year from info if possible, usually first item
		parts := strings.Split(info, "\n")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if len(p) == 4 && regexp.MustCompile(`^\d{4}$`).MatchString(p) {
				year = p
				break
			}
		}

		if href != "" && title != "" {
			results = append(results, core.SearchResult{
				Title:  title,
				URL:    BRAFLIX_BASE_URL + href,
				Type:   mediaType,
				Poster: poster,
				Year:   year,
			})
		}
	})

	if len(results) == 0 {
		return nil, errors.New("no results")
	}

	return results, nil
}

func (b *Braflix) GetMediaID(urlStr string) (string, error) {
	// Extract ID from URL: ...-19722 or ...-19722.5376856 (if on watch page)
	// Usually ends with digits
	re := regexp.MustCompile(`-(\d+)$`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) > 1 {
		return matches[1], nil
	}
	// Try parsing from middle if slug has numbers
	// Example: /movie/watch-avengers-endgame-movies-free-braflix-19722
	// Split by "-" and take last
	parts := strings.Split(urlStr, "-")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// Remove potential trailing slash or query params
		if idx := strings.Index(last, "?"); idx != -1 {
			last = last[:idx]
		}
		if idx := strings.Index(last, "."); idx != -1 { // handle .html or .serverid
			last = last[:idx]
		}
		if _, err := fmt.Sscanf(last, "%d", new(int)); err == nil {
			return last, nil
		}
	}

	return "", errors.New("could not extract media id")
}

func (b *Braflix) GetSeasons(mediaID string) ([]core.Season, error) {
	urlStr := fmt.Sprintf("%s/season/list/%s", BRAFLIX_AJAX_URL, mediaID)
	req, _ := b.newRequest("GET", urlStr, nil)
	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var seasons []core.Season
	doc.Find("a.ss-item").Each(func(i int, s *goquery.Selection) {
		id := s.AttrOr("data-id", "")
		name := strings.TrimSpace(s.Text())
		if id != "" {
			seasons = append(seasons, core.Season{ID: id, Name: name})
		}
	})

	return seasons, nil
}

func (b *Braflix) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	var urlStr string
	if isSeason {
		// id is Season ID
		urlStr = fmt.Sprintf("%s/season/episodes/%s", BRAFLIX_AJAX_URL, id)
	} else {
		// id is Movie ID, return servers as episodes
		urlStr = fmt.Sprintf("%s/episode/list/%s", BRAFLIX_AJAX_URL, id)
	}

	req, _ := b.newRequest("GET", urlStr, nil)
	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var episodes []core.Episode

	if isSeason {
		doc.Find("a.eps-item").Each(func(i int, s *goquery.Selection) {
			epID := s.AttrOr("data-id", "")
			name := strings.TrimSpace(s.AttrOr("title", ""))
			if name == "" {
				name = strings.TrimSpace(s.Text())
			}
			if epID != "" {
				episodes = append(episodes, core.Episode{ID: epID, Name: name})
			}
		})
	} else {
		// Movie: parse servers
		doc.Find("a.link-item").Each(func(i int, s *goquery.Selection) {
			srvID := s.AttrOr("data-id", "") // This is the link_id
			if srvID == "" {
				srvID = s.AttrOr("data-linkid", "") // Sometimes data-linkid
			}
			name := strings.TrimSpace(s.Text())
			// Use span text if available
			if span := s.Find("span").Text(); span != "" {
				name = strings.TrimSpace(span)
			}

			if srvID != "" {
				episodes = append(episodes, core.Episode{ID: srvID, Name: name})
			}
		})
	}

	return episodes, nil
}

func (b *Braflix) GetServers(episodeID string) ([]core.Server, error) {
	urlStr := fmt.Sprintf("%s/episode/servers/%s", BRAFLIX_AJAX_URL, episodeID)
	req, _ := b.newRequest("GET", urlStr, nil)
	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var servers []core.Server
	doc.Find(".link-item").Each(func(i int, s *goquery.Selection) {
		srvID := s.AttrOr("data-id", "")
		name := strings.TrimSpace(s.Find("span").Text())
		if name == "" {
			name = strings.TrimSpace(s.Text())
		}
		if srvID != "" {
			servers = append(servers, core.Server{ID: srvID, Name: name})
		}
	})

	return servers, nil
}

func (b *Braflix) GetLink(serverID string) (string, error) {
	urlStr := fmt.Sprintf("%s/episode/sources/%s", BRAFLIX_AJAX_URL, serverID)
	req, _ := b.newRequest("GET", urlStr, nil)
	resp, err := b.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		Type string `json:"type"`
		Link string `json:"link"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.Link, nil
}
