package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/demonkingswarn/luffy/core"
)

const (
	YOUTUBE_BASE_URL   = "https://www.youtube.com"
	YOUTUBE_SEARCH_URL = YOUTUBE_BASE_URL + "/results"
)

type YouTube struct {
	Client *http.Client
}

func NewYouTube(client *http.Client) *YouTube {
	return &YouTube{Client: client}
}

func (y *YouTube) newRequest(method, url string) (*http.Request, error) {
	req, err := core.NewRequest(method, url)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	return req, nil
}

func (y *YouTube) Search(query string) ([]core.SearchResult, error) {
	v := url.Values{}
	v.Set("search_query", query)
	req, _ := y.newRequest("GET", YOUTUBE_SEARCH_URL+"?"+v.Encode())

	resp, err := y.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	bodyString := string(bodyBytes)

	// Extract ytInitialData
	re := regexp.MustCompile(`var ytInitialData = ({.*?});`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not find ytInitialData")
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(matches[1]), &data); err != nil {
		return nil, err
	}

	var results []core.SearchResult

	// Navigate JSON: contents -> twoColumnSearchResultsRenderer -> primaryContents -> sectionListRenderer -> contents
	contents, _ := data["contents"].(map[string]interface{})
	twoCol, _ := contents["twoColumnSearchResultsRenderer"].(map[string]interface{})
	primary, _ := twoCol["primaryContents"].(map[string]interface{})
	sectionList, _ := primary["sectionListRenderer"].(map[string]interface{})
	secContents, _ := sectionList["contents"].([]interface{})

	for _, sec := range secContents {
		secMap, _ := sec.(map[string]interface{})
		itemSec, _ := secMap["itemSectionRenderer"].(map[string]interface{})
		items, _ := itemSec["contents"].([]interface{})

		for _, item := range items {
			itemMap, _ := item.(map[string]interface{})
			if videoRenderer, ok := itemMap["videoRenderer"].(map[string]interface{}); ok {
				videoId, _ := videoRenderer["videoId"].(string)

				titleObj, _ := videoRenderer["title"].(map[string]interface{})
				titleRuns, _ := titleObj["runs"].([]interface{})
				title := ""
				if len(titleRuns) > 0 {
					run, _ := titleRuns[0].(map[string]interface{})
					title, _ = run["text"].(string)
				}

				thumbObj, _ := videoRenderer["thumbnail"].(map[string]interface{})
				thumbs, _ := thumbObj["thumbnails"].([]interface{})
				poster := ""
				if len(thumbs) > 0 {
					t, _ := thumbs[0].(map[string]interface{})
					poster, _ = t["url"].(string)
				}

				if videoId != "" {
					results = append(results, core.SearchResult{
						Title:  title,
						URL:    YOUTUBE_BASE_URL + "/watch?v=" + videoId,
						Type:   core.Movie, // Treat as movie for simplicity
						Poster: poster,
					})
				}
			}
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	return results, nil
}

func (y *YouTube) GetMediaID(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if v := q.Get("v"); v != "" {
		return v, nil
	}
	// Handle shortened urls like youtu.be/ID
	if u.Host == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/"), nil
	}
	return "", fmt.Errorf("could not extract video ID")
}

func (y *YouTube) GetSeasons(mediaID string) ([]core.Season, error) {
	return []core.Season{{ID: "1", Name: "Video"}}, nil
}

func (y *YouTube) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	return []core.Episode{{ID: id, Name: "Watch Video"}}, nil
}

func (y *YouTube) GetServers(episodeID string) ([]core.Server, error) {
	return []core.Server{{ID: episodeID, Name: "YouTube"}}, nil
}

func (y *YouTube) GetLink(serverID string) (string, error) {
	return YOUTUBE_BASE_URL + "/watch?v=" + serverID, nil
}
