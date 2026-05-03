package providers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/demonkingswarn/luffy/core"
)

const (
	HDREZKA_BASE_URL       = "https://hdrezka.website"
	HDREZKA_AJAX_URL       = "https://hdrezka.website/ajax/get_cdn_series/"
	HDREZKA_AJAX_MOVIE_URL = "https://hdrezka.website/ajax/get_cdn_movie/"
)

type HDRezka struct {
	Client *http.Client
}

func NewHDRezka(client *http.Client) *HDRezka {
	return &HDRezka{Client: client}
}

func (h *HDRezka) newRequest(method, urlStr string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", HDREZKA_BASE_URL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}

	return req, nil
}

func (h *HDRezka) Search(query string) ([]core.SearchResult, error) {
	searchURL := fmt.Sprintf("%s/search/?q=%s", HDREZKA_BASE_URL, url.QueryEscape(query))

	req, _ := h.newRequest("GET", searchURL, nil)
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, _ := goquery.NewDocumentFromReader(resp.Body)

	var results []core.SearchResult

	doc.Find("div.b-content__inline_item").Each(func(i int, s *goquery.Selection) {
		link := s.Find("div.b-content__inline_item-link a")
		title := strings.TrimSpace(link.Text())
		href := link.AttrOr("href", "")
		poster := s.Find("img").AttrOr("src", "")
		misc := s.Find("div.misc").Text()

		mediaType := core.Movie
		if s.Find("span.cat.series").Length() > 0 {
			mediaType = core.Series
		}

		if href != "" {
			results = append(results, core.SearchResult{
				Title:  title,
				URL:    href,
				Type:   mediaType,
				Poster: poster,
				Year:   misc,
			})
		}
	})

	if len(results) == 0 {
		return nil, errors.New("no results")
	}

	return results, nil
}

func (h *HDRezka) GetMediaID(urlStr string) (string, error) {
	if strings.HasPrefix(urlStr, "/") {
		urlStr = HDREZKA_BASE_URL + urlStr
	}
	return urlStr, nil
}

func (h *HDRezka) GetSeasons(mediaID string) ([]core.Season, error) {
	req, _ := h.newRequest("GET", mediaID, nil)
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, _ := goquery.NewDocumentFromReader(resp.Body)

	var seasons []core.Season
	doc.Find("ul.b-simple_seasons__list li").Each(func(i int, s *goquery.Selection) {
		id := s.AttrOr("data-tab_id", "")
		name := strings.TrimSpace(s.Text())
		if id != "" {
			seasons = append(seasons, core.Season{ID: mediaID + "|" + id, Name: name})
		}
	})

	if len(seasons) == 0 {
		seasons = append(seasons, core.Season{ID: mediaID + "|1", Name: "Season 1"})
	}

	return seasons, nil
}

func (h *HDRezka) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	parts := strings.Split(id, "|")
	urlStr := parts[0]
	seasonID := "1"
	if len(parts) >= 2 && parts[1] != "" {
		seasonID = parts[1]
	}

	req, _ := h.newRequest("GET", urlStr, nil)
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, _ := goquery.NewDocumentFromReader(resp.Body)

	var episodes []core.Episode

	if isSeason {
		doc.Find(fmt.Sprintf("ul#simple-episodes-list-%s li", seasonID)).Each(func(i int, s *goquery.Selection) {
			epId := s.AttrOr("data-episode_id", strconv.Itoa(i+1))
			name := strings.TrimSpace(s.Text())
			episodes = append(episodes, core.Episode{
				ID:   fmt.Sprintf("%s|%s|%s", urlStr, seasonID, epId),
				Name: name,
			})
		})
	} else {
		// For movies, return one entry per translator (server) so the caller
		// can call GetLink() directly with a server-like ID.
		doc.Find("ul#translators-list li").Each(func(i int, s *goquery.Selection) {
			tID := s.AttrOr("data-translator_id", "")
			epId := s.AttrOr("data-episode_id", "1")
			name := strings.TrimSpace(s.Text())
			if tID != "" {
				episodes = append(episodes, core.Episode{
					ID:   fmt.Sprintf("%s|%s|%s|%s", urlStr, seasonID, epId, tID),
					Name: name,
				})
			}
		})
	}

	if len(episodes) == 0 {
		// Fallback: single full-movie entry (older behavior)
		episodes = append(episodes, core.Episode{
			ID:   fmt.Sprintf("%s|1|1", urlStr),
			Name: "Full Movie",
		})
	}

	return episodes, nil
}

func (h *HDRezka) GetServers(episodeID string) ([]core.Server, error) {
	parts := strings.Split(episodeID, "|")
	if len(parts) < 3 {
		return nil, errors.New("invalid id")
	}

	urlStr, season, episode := parts[0], parts[1], parts[2]

	req, _ := h.newRequest("GET", urlStr, nil)
	resp, _ := h.Client.Do(req)
	defer resp.Body.Close()

	doc, _ := goquery.NewDocumentFromReader(resp.Body)

	var servers []core.Server
	doc.Find("ul#translators-list li").Each(func(i int, s *goquery.Selection) {
		tID := s.AttrOr("data-translator_id", "")
		name := strings.TrimSpace(s.Text())

		if tID != "" {
			servers = append(servers, core.Server{
				ID:   fmt.Sprintf("%s|%s|%s|%s", urlStr, season, episode, tID),
				Name: name,
			})
		}
	})

	if len(servers) == 0 {
		servers = append(servers, core.Server{
			ID:   fmt.Sprintf("%s|%s|%s|238", urlStr, season, episode),
			Name: "Default",
		})
	}

	return servers, nil
}

func (h *HDRezka) GetLink(serverID string) (string, error) {
	parts := strings.Split(serverID, "|")
	if len(parts) < 4 {
		return "", errors.New("invalid server id")
	}

	urlStr, translatorID := parts[0], parts[3]

	re := regexp.MustCompile(`/(\d+)-[^/]+\.html`)
	match := re.FindStringSubmatch(urlStr)
	if len(match) < 2 {
		return "", errors.New("failed to extract id")
	}

	id := match[1]

	// tryGetLink attempts the AJAX call for a given translator id with retries
	tryGetLink := func(tid string) (string, error) {
		vals := url.Values{}
		vals.Set("id", id)
		vals.Set("translator_id", tid)
		vals.Set("is_camrip", "0")
		vals.Set("is_ads", "0")
		vals.Set("is_director", "0")

		endpoint := HDREZKA_AJAX_URL
		if strings.Contains(urlStr, "/films/") {
			endpoint = HDREZKA_AJAX_MOVIE_URL
		}

		// Retry a few times for transient 5xx/errors with exponential backoff
		var lastErr error
		for attempt := 0; attempt < 5; attempt++ {
			bodyStr := vals.Encode()
			req, _ := h.newRequest("POST", endpoint, strings.NewReader(bodyStr))
			req.Header.Set("Referer", urlStr)

			// perform request
			resp, err := h.Client.Do(req)
			if err != nil {
				lastErr = err
				time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// write debug log for this attempt
			var logb strings.Builder
			logb.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format(time.RFC3339)))
			logb.WriteString(fmt.Sprintf("Translator: %s\n", tid))
			logb.WriteString(fmt.Sprintf("Attempt: %d\n", attempt+1))
			logb.WriteString("--- REQUEST ---\n")
			logb.WriteString(fmt.Sprintf("POST %s\n", endpoint))
			for k, vv := range req.Header {
				logb.WriteString(fmt.Sprintf("%s: %s\n", k, strings.Join(vv, ",")))
			}
			logb.WriteString("\n")
			logb.WriteString(bodyStr + "\n")
			logb.WriteString("--- RESPONSE ---\n")
			logb.WriteString(fmt.Sprintf("Status: %s\n", resp.Status))
			for k, vv := range resp.Header {
				logb.WriteString(fmt.Sprintf("%s: %s\n", k, strings.Join(vv, ",")))
			}
			logb.WriteString("\n")
			logb.WriteString(strings.TrimSpace(string(body)) + "\n")
			// write to file (overwrite)
			_ = os.WriteFile("hdrezka_debug.log", []byte(logb.String()), 0644)

			// If server returned HTML (e.g., 503 page), return a helpful error
			bstr := strings.TrimSpace(string(body))
			if len(bstr) > 0 && bstr[0] == '<' {
				lastErr = fmt.Errorf("html error: %s", bstr)
				if resp.StatusCode >= 500 {
					time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
					continue
				}
				return "", lastErr
			}

			var res struct {
				Success bool   `json:"success"`
				URL     string `json:"url"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(body, &res); err != nil {
				lastErr = fmt.Errorf("json error: %s", strings.TrimSpace(string(body)))
				// don't retry on parse errors
				return "", lastErr
			}
			if res.Success && res.URL != "" {
				return decode(res.URL), nil
			}
			lastErr = fmt.Errorf("failed: %s", strings.TrimSpace(string(body)))
			// retry briefly for transient failures
			if resp.StatusCode >= 500 {
				time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
				continue
			}
			return "", lastErr
		}
		return "", lastErr
	}

	// First, try the provided translator id
	if translatorID != "" {
		if link, err := tryGetLink(translatorID); err == nil {
			return link, nil
		}
	}

	// Fallback: fetch the page and iterate available translators
	req, _ := h.newRequest("GET", urlStr, nil)
	resp, err := h.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, _ := goquery.NewDocumentFromReader(resp.Body)
	tried := map[string]bool{}
	var resolved string
	// try translators listed on the page
	doc.Find("ul#translators-list li").EachWithBreak(func(i int, s *goquery.Selection) bool {
		tID := s.AttrOr("data-translator_id", "")
		if tID == "" || tried[tID] {
			return true
		}
		tried[tID] = true
		if link, err := tryGetLink(tID); err == nil {
			link = strings.TrimSpace(link)
			if link != "" {
				resolved = link
				return false // stop iteration
			}
		}
		return true
	})

	if resolved != "" {
		return resolved, nil
	}

	// If we reach here, none worked — return a generic error
	return "", fmt.Errorf("could not resolve stream for %s", urlStr)
}

func decode(data string) string {
	if strings.HasPrefix(data, "#h") {
		data = data[2:]
	}

	chunks := strings.Split(data, "//_//")
	var result strings.Builder

	for _, chunk := range chunks {
		decoded, err := base64.StdEncoding.DecodeString(chunk)
		if err == nil && len(decoded) > 10 {
			result.WriteString(string(decoded))
		}
	}

	return result.String()
}
