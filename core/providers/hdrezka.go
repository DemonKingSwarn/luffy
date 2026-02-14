package providers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

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
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, nil
}

func (h *HDRezka) Search(query string) ([]core.SearchResult, error) {
	encodedQuery := url.QueryEscape(query)
	searchURL := fmt.Sprintf("%s/search/?q=%s", HDREZKA_BASE_URL, encodedQuery)

	req, _ := h.newRequest("GET", searchURL, nil)
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []core.SearchResult

	doc.Find("div.b-content__inline_item").Each(func(i int, s *goquery.Selection) {
		link := s.Find("div.b-content__inline_item-link a")
		title := link.Text()
		href := link.AttrOr("href", "")
		poster := s.Find("img").AttrOr("src", "")
		misc := s.Find("div.misc").Text()

		mediaType := core.Movie
		if strings.Contains(s.Find("span.cat").AttrOr("class", ""), "series") ||
			strings.Contains(s.Find("span.cat").AttrOr("class", ""), "cartoons") ||
			strings.Contains(s.Find("span.cat").AttrOr("class", ""), "animation") {
			if strings.Contains(s.Find("span.info").Text(), "сезон") || strings.Contains(s.Find("span.info").Text(), "серия") {
				mediaType = core.Series
			}
		}
		if s.Find("span.cat.series").Length() > 0 {
			mediaType = core.Series
		}

		if href != "" {
			results = append(results, core.SearchResult{
				Title:  strings.TrimSpace(title),
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

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

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
	if len(parts) < 2 {
		return nil, errors.New("invalid id format")
	}
	urlStr := parts[0]
	seasonID := parts[1]

	req, _ := h.newRequest("GET", urlStr, nil)
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var episodes []core.Episode
	listSelector := fmt.Sprintf("ul#simple-episodes-list-%s li", seasonID)

	doc.Find(listSelector).Each(func(i int, s *goquery.Selection) {
		epId := s.AttrOr("data-episode_id", "")
		name := strings.TrimSpace(s.Text())
		if epId == "" {
			epId = strconv.Itoa(i + 1)
		}
		compositeID := fmt.Sprintf("%s|%s|%s", urlStr, seasonID, epId)
		episodes = append(episodes, core.Episode{ID: compositeID, Name: name})
	})

	if len(episodes) == 0 {
		compositeID := fmt.Sprintf("%s|%s|%s", urlStr, "1", "1")
		episodes = append(episodes, core.Episode{ID: compositeID, Name: "Full Movie"})
	}

	return episodes, nil
}

func (h *HDRezka) GetServers(episodeID string) ([]core.Server, error) {
	parts := strings.Split(episodeID, "|")
	if len(parts) < 3 {
		return nil, errors.New("invalid episode id")
	}
	urlStr := parts[0]
	season := parts[1]
	episode := parts[2]

	req, _ := h.newRequest("GET", urlStr, nil)
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var servers []core.Server
	doc.Find("ul#translators-list li").Each(func(i int, s *goquery.Selection) {
		tID := s.AttrOr("data-translator_id", "")
		name := strings.TrimSpace(s.Text())
		if s.HasClass("b-prem_translator") {
			name += " (Premium)"
		}

		if tID != "" {
			srvID := fmt.Sprintf("%s|%s|%s|%s", urlStr, season, episode, tID)
			servers = append(servers, core.Server{ID: srvID, Name: name})
		}
	})

	if len(servers) == 0 {
		html, _ := doc.Html()
		re := regexp.MustCompile(`initCDNSeriesEvents\(\d+,\s*(\d+)`)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			tID := matches[1]
			srvID := fmt.Sprintf("%s|%s|%s|%s", urlStr, season, episode, tID)
			servers = append(servers, core.Server{ID: srvID, Name: "Default"})
		} else {
			reMovie := regexp.MustCompile(`initCDNMoviesEvents\(\d+,\s*(\d+)`)
			matchesMovie := reMovie.FindStringSubmatch(html)
			if len(matchesMovie) > 1 {
				tID := matchesMovie[1]
				srvID := fmt.Sprintf("%s|%s|%s|%s", urlStr, season, episode, tID)
				servers = append(servers, core.Server{ID: srvID, Name: "Default"})
			}
		}
	}

	return servers, nil
}

func (h *HDRezka) GetLink(serverID string) (string, error) {
	parts := strings.Split(serverID, "|")
	if len(parts) < 4 {
		return "", errors.New("invalid server id")
	}
	urlStr := parts[0]
	season := parts[1]
	episode := parts[2]
	translatorID := parts[3]

	re := regexp.MustCompile(`\/(\d+)-`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) < 2 {
		return "", errors.New("could not extract id from url")
	}
	id := matches[1]

	action := "get_stream"
	endpoint := HDREZKA_AJAX_URL

	if strings.Contains(urlStr, "/films/") {
		action = "get_movie_stream"
		endpoint = HDREZKA_AJAX_MOVIE_URL
	}

	vals := url.Values{}
	vals.Set("id", id)
	vals.Set("translator_id", translatorID)
	vals.Set("action", action)

	if action == "get_stream" {
		vals.Set("season", season)
		vals.Set("episode", episode)
	}

	req, _ := h.newRequest("POST", endpoint, strings.NewReader(vals.Encode()))
	req.Header.Set("Referer", urlStr)

	resp, err := h.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		URL     string `json:"url"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("json error: %v body: %s", err, string(body))
	}

	if !res.Success {
		if action == "get_stream" {
			vals.Set("action", "get_movie_stream")
			req, _ := h.newRequest("POST", HDREZKA_AJAX_MOVIE_URL, strings.NewReader(vals.Encode()))
			req.Header.Set("Referer", urlStr)
			resp2, err := h.Client.Do(req)
			if err == nil {
				defer resp2.Body.Close()
				body2, _ := io.ReadAll(resp2.Body)
				json.Unmarshal(body2, &res)
			}
		}
	}

	if !res.Success {
		return "", fmt.Errorf("api error: %s", res.Message)
	}

	return h.Decode(res.URL), nil
}

func (h *HDRezka) Decode(data string) string {
	if strings.HasPrefix(data, "#h") {
		data = data[2:]
	}

	chunks := strings.Split(data, "//_//")
	var valid strings.Builder

	for _, chunk := range chunks {
		decoded, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil {
			continue
		}

		decodedStr := string(decoded)
		if strings.ContainsAny(decodedStr, "#!@$^%") {
			continue
		}

		valid.WriteString(decodedStr)
	}

	return valid.String()
}
