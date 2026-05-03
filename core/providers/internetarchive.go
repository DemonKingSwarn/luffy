package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/demonkingswarn/luffy/core"
)

const (
	IA_SEARCH_URL   = "https://archive.org/advancedsearch.php"
	IA_METADATA_URL = "https://archive.org/metadata/"
	IA_BASE_URL     = "https://archive.org"
)

type InternetArchive struct {
	Client *http.Client
}

func NewInternetArchive(client *http.Client) *InternetArchive {
	return &InternetArchive{Client: client}
}

func (ia *InternetArchive) newRequest(method, urlStr string) (*http.Request, error) {
	req, err := core.NewRequest(method, urlStr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://archive.org")
	return req, nil
}

// Search uses archive.org advancedsearch to find movie items.
func (ia *InternetArchive) Search(query string) ([]core.SearchResult, error) {
	q := fmt.Sprintf("title:(%s) AND mediatype:(movies)", query)
	vals := url.Values{}
	vals.Set("q", q)
	vals.Set("fl", "identifier,title,year")
	vals.Set("rows", "20")
	vals.Set("output", "json")

	req, _ := ia.newRequest("GET", IA_SEARCH_URL+"?"+vals.Encode())
	resp, err := ia.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Response struct {
			Docs []struct {
				Identifier string      `json:"identifier"`
				Title      string      `json:"title"`
				Year       interface{} `json:"year"`
			} `json:"docs"`
		} `json:"response"`
	}
	// Read raw body to help debugging type inconsistencies.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile("ia_search_debug.json", bodyBytes, 0644)
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, err
	}

	var results []core.SearchResult
	for _, d := range res.Response.Docs {
		if d.Identifier == "" {
			continue
		}
		yearStr := ""
		switch y := d.Year.(type) {
		case string:
			yearStr = y
		case float64:
			yearStr = fmt.Sprintf("%d", int(y))
		}
		results = append(results, core.SearchResult{
			Title:  d.Title,
			URL:    IA_BASE_URL + "/details/" + d.Identifier,
			Type:   core.Movie,
			Poster: "",
			Year:   yearStr,
		})
	}
	if len(results) == 0 {
		return nil, errors.New("no results")
	}
	return results, nil
}

func (ia *InternetArchive) GetMediaID(urlStr string) (string, error) {
	// expect url like https://archive.org/details/{identifier}
	parts := strings.Split(strings.TrimRight(urlStr, "/"), "/")
	if len(parts) == 0 {
		return "", errors.New("invalid url")
	}
	id := parts[len(parts)-1]
	if id == "" {
		return "", errors.New("invalid id")
	}
	return id, nil
}

func (ia *InternetArchive) GetSeasons(mediaID string) ([]core.Season, error) {
	// movies only
	return nil, nil
}

func (ia *InternetArchive) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	// For movies, return each video file as an episode/server entry.
	// Fetch metadata
	req, _ := ia.newRequest("GET", IA_METADATA_URL+id)
	resp, err := ia.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var meta struct {
		Files []struct {
			Name   string `json:"name"`
			Format string `json:"format"`
		} `json:"files"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}

	var eps []core.Episode
	for _, f := range meta.Files {
		lname := strings.ToLower(f.Name)
		if strings.HasSuffix(lname, ".mp4") || strings.HasSuffix(lname, ".webm") || strings.HasSuffix(lname, ".ogv") {
			eps = append(eps, core.Episode{ID: fmt.Sprintf("%s|%s", id, f.Name), Name: f.Name})
		}
	}
	if len(eps) == 0 {
		// fallback: one entry pointing to the details page
		eps = append(eps, core.Episode{ID: fmt.Sprintf("%s|%s", id, ""), Name: meta.Title})
	}
	return eps, nil
}

func (ia *InternetArchive) GetServers(episodeID string) ([]core.Server, error) {
	parts := strings.Split(episodeID, "|")
	if len(parts) < 1 {
		return nil, errors.New("invalid id")
	}
	id := parts[0]
	filename := ""
	if len(parts) >= 2 {
		filename = parts[1]
	}

	if filename == "" {
		// expose the details page as a single server
		return []core.Server{{ID: id, Name: "archive"}}, nil
	}
	return []core.Server{{ID: fmt.Sprintf("%s|%s", id, filename), Name: filename}}, nil
}

func (ia *InternetArchive) GetLink(serverID string) (string, error) {
	parts := strings.Split(serverID, "|")
	if len(parts) < 1 {
		return "", errors.New("invalid id")
	}
	id := parts[0]
	filename := ""
	if len(parts) >= 2 {
		filename = parts[1]
	}
	if filename == "" {
		return IA_BASE_URL + "/details/" + id, nil
	}
	return fmt.Sprintf("https://archive.org/download/%s/%s", id, url.PathEscape(filename)), nil
}
