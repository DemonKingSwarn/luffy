package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const TMDB_API_KEY = ""

type tmdbSearchResponse struct {
	Results []struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Year  string `json:"release_date"`
	} `json:"results"`
}

func GetTMDBMovieID(query string, client *http.Client) (int, error) {
	endpoint := fmt.Sprintf(
		"https://api.themoviedb.org/3/search/movie?api_key=%s&query=%s",
		TMDB_API_KEY,
		url.QueryEscape(query),
	)

	for i := 0; i < 5; i++ {
		tr := &http.Transport{
			DisableKeepAlives: true,
		}
		client.Transport = tr

		req, _ := http.NewRequest("GET", endpoint, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(time.Second * time.Duration(i+2))
			continue
		}
		defer resp.Body.Close()

		var data tmdbSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			time.Sleep(time.Second * time.Duration(i+2))
			continue
		}

		if len(data.Results) == 0 {
			return 0, fmt.Errorf("no TMDB results")
		}

		return data.Results[0].ID, nil
	}
	return 0, fmt.Errorf("TMDB request failed after retries")
}
