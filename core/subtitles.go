package core

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// FetchOpenSubtitles fetches English subtitles from OpenSubtitles legacy API using an IMDB ID.
// Supports season and episode filters for TV shows to ensure precise subtitle timing.
// Downloads and decompresses the subtitle locally to prevent player and compression issues.
// Returns up to 1 local subtitle file path. Falls back gracefully on any error.
func FetchOpenSubtitles(imdbID string, season, episode int, client *http.Client) []string {
	if imdbID == "" {
		return nil
	}
	// Strip "tt" prefix if present
	imdbID = strings.TrimPrefix(imdbID, "tt")

	var apiURL string
	if season > 0 && episode > 0 {
		apiURL = fmt.Sprintf("https://rest.opensubtitles.org/search/episode-%d/imdbid-%s/season-%d/sublanguageid-eng", episode, imdbID, season)
	} else {
		apiURL = fmt.Sprintf("https://rest.opensubtitles.org/search/imdbid-%s/sublanguageid-eng", imdbID)
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "TemporaryUserAgent")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var results []struct {
		SubDownloadLink    string `json:"SubDownloadLink"`
		SubFormat          string `json:"SubFormat"`
		SubHearingImpaired string `json:"SubHearingImpaired"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil
	}

	var chosenURL string
	// Return first non-HI srt subtitle
	for _, r := range results {
		if r.SubDownloadLink != "" && r.SubFormat == "srt" && r.SubHearingImpaired == "0" {
			chosenURL = r.SubDownloadLink
			break
		}
	}
	// Fallback to first available
	if chosenURL == "" && len(results) > 0 && results[0].SubDownloadLink != "" {
		chosenURL = results[0].SubDownloadLink
	}

	if chosenURL != "" {
		localPath, err := downloadAndDecompressGzip(chosenURL, client)
		if err == nil {
			return []string{localPath}
		}
	}

	return nil
}

func downloadAndDecompressGzip(subURL string, client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", subURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "TemporaryUserAgent")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download subtitle: status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if strings.HasSuffix(strings.ToLower(subURL), ".gz") || resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	tempFile, err := os.CreateTemp("", "luffy-sub-*.srt")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, reader)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

// GetIMDBIDFromTMDB fetches the IMDB ID for a given TMDB ID and media type.
func GetIMDBIDFromTMDB(tmdbID, mediaType string, client *http.Client) string {
	if tmdbID == "" {
		return ""
	}
	if mediaType == "" {
		mediaType = "movie"
	}
	// TMDB API expects "tv" instead of "series"
	if mediaType == "series" {
		mediaType = "tv"
	}
	apiURL := fmt.Sprintf("%s/%s/%s/external_ids?api_key=%s", TMDB_BASE_URL, mediaType, tmdbID, TMDB_API_KEY)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var data struct {
		IMDBID string `json:"imdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	return data.IMDBID
}

// GetIMDBIDByTitle searches TMDB by title to find the IMDB ID.
func GetIMDBIDByTitle(title string, mediaType MediaType, client *http.Client) string {
	if title == "" {
		return ""
	}
	tmdbMediaType := string(mediaType)
	if tmdbMediaType == "series" {
		tmdbMediaType = "tv"
	}

	endpoint := fmt.Sprintf(
		"%s/search/multi?api_key=%s&query=%s",
		TMDB_BASE_URL,
		TMDB_API_KEY,
		url.QueryEscape(title),
	)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var data struct {
		Results []struct {
			ID        int    `json:"id"`
			MediaType string `json:"media_type"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}

	for _, r := range data.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			if tmdbMediaType != "" && r.MediaType != tmdbMediaType {
				continue
			}
			imdbID := GetIMDBIDFromTMDB(fmt.Sprintf("%d", r.ID), r.MediaType, client)
			if imdbID != "" {
				return imdbID
			}
		}
	}

	for _, r := range data.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			imdbID := GetIMDBIDFromTMDB(fmt.Sprintf("%d", r.ID), r.MediaType, client)
			if imdbID != "" {
				return imdbID
			}
		}
	}

	return ""
}

// ExtractTMDBIDFromURL tries to extract a TMDB ID from provider URLs.
// Supports patterns like /movie/123, /tv/123, /embed/movie/123, /embed/tv/123/1/1
func ExtractTMDBIDFromURL(rawURL string) (tmdbID, mediaType string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if (p == "movie" || p == "tv") && i+1 < len(parts) {
			id := parts[i+1]
			// Verify it looks like a numeric ID
			if len(id) > 0 && id[0] >= '0' && id[0] <= '9' {
				return id, p
			}
		}
	}
	return "", ""
}
