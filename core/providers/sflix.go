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
	SFLIX_BASE_URL   = "https://sflix.is"
	SFLIX_SEARCH_URL = SFLIX_BASE_URL + "/search"
	SFLIX_AJAX_URL   = SFLIX_BASE_URL + "/ajax"
)

type Sflix struct {
	Client *http.Client
}

func NewSflix(client *http.Client) *Sflix {
	return &Sflix{Client: client}
}

func (s *Sflix) newRequest(method, url string) (*http.Request, error) {
	req, err := core.NewRequest(method, url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", SFLIX_BASE_URL+"/")
	return req, nil
}

func (s *Sflix) Search(query string) ([]core.SearchResult, error) {
	search := strings.ReplaceAll(query, " ", "-")
	req, _ := s.newRequest("GET", SFLIX_SEARCH_URL+"/"+search)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []core.SearchResult

	doc.Find("div.flw-item").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		if i >= 10 {
			return false
		}

		title := sel.Find("h2.film-name a").AttrOr("title", "Unknown")
		href := sel.Find("div.film-poster a").AttrOr("href", "")
		poster := sel.Find("img.film-poster-img").AttrOr("data-src", "")
		// Get type from the strong tag inside fdi-item (e.g., "TV" or "Movie")
		typeStr := strings.TrimSpace(sel.Find("span.fdi-item strong").Text())

		mediaType := core.Movie
		if strings.EqualFold(typeStr, "TV") || strings.EqualFold(typeStr, "Series") {
			mediaType = core.Series
		}

		// Also check URL path as fallback (/tv/ vs /movie/)
		if strings.Contains(href, "/tv/") {
			mediaType = core.Series
		} else if strings.Contains(href, "/movie/") {
			mediaType = core.Movie
		}

		if href != "" {
			results = append(results, core.SearchResult{
				Title:  title,
				URL:    SFLIX_BASE_URL + href,
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

func (s *Sflix) GetMediaID(url string) (string, error) {
	req, _ := s.newRequest("GET", url)
	resp, err := s.Client.Do(req)
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

func (s *Sflix) GetSeasons(mediaID string) ([]core.Season, error) {
	// Parse mediaID format: "id" or "id|type" or "id|type|extra"
	var actualMediaID, mediaType string
	if strings.Contains(mediaID, "|") {
		parts := strings.Split(mediaID, "|")
		actualMediaID = parts[0]
		if len(parts) > 1 {
			mediaType = parts[1]
		}
	} else {
		actualMediaID = mediaID
	}

	url := fmt.Sprintf("%s/season/list/%s", SFLIX_AJAX_URL, actualMediaID)
	req, _ := s.newRequest("GET", url)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var seasons []core.Season
	doc.Find(".dropdown-item, .ss-item").Each(func(i int, sel *goquery.Selection) {
		id := sel.AttrOr("data-id", "")
		name := strings.TrimSpace(sel.Text())
		if id != "" {
			// Append mediaID and type to season ID for context
			// Format: "seasonID|mediaID|type"
			if mediaID != "" {
				if mediaType != "" {
					id = id + "|" + actualMediaID + "|" + mediaType
				} else {
					id = id + "|" + actualMediaID
				}
			}
			seasons = append(seasons, core.Season{ID: id, Name: name})
		}
	})
	return seasons, nil
}

func (s *Sflix) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	// Parse ID format: "id" or "id|mediaID" or "id|mediaID|type"
	var actualID, mediaID, mediaType string
	parts := strings.Split(id, "|")
	if len(parts) >= 2 {
		actualID = parts[0]
		mediaID = parts[1]
		if len(parts) >= 3 {
			mediaType = parts[2]
		}
	} else {
		actualID = id
	}

	var url string
	if isSeason {
		url = fmt.Sprintf("%s/season/episodes/%s", SFLIX_AJAX_URL, actualID)
	} else {
		url = fmt.Sprintf("%s/episode/list/%s", SFLIX_AJAX_URL, actualID)
	}

	req, _ := s.newRequest("GET", url)
	resp, err := s.Client.Do(req)
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
		doc.Find(".eps-item").Each(func(i int, sel *goquery.Selection) {
			epID := sel.AttrOr("data-id", "")
			// Get title from the img element inside
			name := strings.TrimSpace(sel.Find("img.film-poster-img").AttrOr("title", ""))
			if name == "" {
				name = strings.TrimSpace(sel.Text())
			}
			if epID != "" {
				// Append mediaID and type to episode ID for context
				// Format: "episodeID|mediaID|type"
				if mediaID != "" {
					if mediaType != "" {
						epID = epID + "|" + mediaID + "|" + mediaType
					} else {
						epID = epID + "|" + mediaID
					}
				}
				episodes = append(episodes, core.Episode{ID: epID, Name: name})
			}
		})
	} else {
		// Movies: List of servers (treated as episodes/servers)
		doc.Find(".link-item").Each(func(i int, sel *goquery.Selection) {
			epID := sel.AttrOr("data-id", "")
			name := strings.TrimSpace(sel.Find("span").Text())
			if epID != "" {
				// Append mediaID and type to episode ID for context
				// Format: "serverID|mediaID|type"
				if mediaID != "" {
					if mediaType != "" {
						epID = epID + "|" + mediaID + "|" + mediaType
					} else {
						epID = epID + "|" + mediaID
					}
				}
				episodes = append(episodes, core.Episode{ID: epID, Name: name})
			}
		})
	}

	return episodes, nil
}

func (s *Sflix) GetServers(episodeID string) ([]core.Server, error) {
	// Parse episodeID format: "id" or "id|mediaID" or "id|mediaID|type"
	var actualEpisodeID, mediaID, mediaType string
	parts := strings.Split(episodeID, "|")
	if len(parts) >= 2 {
		actualEpisodeID = parts[0]
		mediaID = parts[1]
		if len(parts) >= 3 {
			mediaType = parts[2]
		}
	} else {
		actualEpisodeID = episodeID
	}

	return s.fetchServersWithMediaID(actualEpisodeID, mediaID, mediaType)
}

func (s *Sflix) fetchServersWithMediaID(episodeID string, mediaID string, mediaType string) ([]core.Server, error) {
	// Determine endpoint based on whether it's a movie or TV show
	var endpoint string
	var isMovie bool

	// Check mediaType first
	if mediaType != "" {
		isMovie = strings.EqualFold(mediaType, "movie")
	} else {
		// Fall back to checking mediaID string content
		isMovie = strings.Contains(mediaID, "movie") || !strings.Contains(mediaID, "tv")
	}

	if isMovie {
		// For movies, use /ajax/episode/list/{episodeId}
		endpoint = fmt.Sprintf("%s/episode/list/%s", SFLIX_AJAX_URL, episodeID)
	} else {
		// For TV shows, use /ajax/episode/servers/{episodeId}
		endpoint = fmt.Sprintf("%s/episode/servers/%s", SFLIX_AJAX_URL, episodeID)
	}

	req, _ := s.newRequest("GET", endpoint)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var servers []core.Server

	// Find all server items
	doc.Find(".link-item, .ulclear > li").Each(func(i int, sel *goquery.Selection) {
		dataID, exists := sel.Attr("data-id")
		if !exists {
			dataID = sel.Find("a").AttrOr("data-id", "")
		}

		// Server name is in <span> tag or directly in the link
		serverName := strings.TrimSpace(sel.Find("span").Text())
		if serverName == "" {
			serverName = strings.TrimSpace(sel.Find("a span").Text())
		}
		if serverName == "" {
			serverName = strings.TrimSpace(sel.Text())
		}
		if dataID != "" && serverName != "" {
			servers = append(servers, core.Server{
				ID:   dataID,
				Name: serverName,
			})
		}
	})

	return servers, nil
}

func (s *Sflix) GetLink(serverID string) (string, error) {
	id := serverID
	if idx := strings.Index(id, "|"); idx != -1 {
		id = id[:idx]
	}
	url := fmt.Sprintf("%s/episode/sources/%s", SFLIX_AJAX_URL, id)
	req, _ := s.newRequest("GET", url)
	resp, err := s.Client.Do(req)
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

// ExtractM3U8 extracts the m3u8 URL from an embed link
// This follows the pattern from the example file's extractSourcesFromServer
func (s *Sflix) ExtractM3U8(embedURL string) (string, []string, string, error) {
	// Use the core DecryptStream function to extract m3u8
	return core.DecryptStream(embedURL, s.Client)
}
