package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type DecryptedSource struct {
	File  string `json:"file"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type DecryptedTrack struct {
	File  string `json:"file"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type DecryptResponse struct {
	Sources []DecryptedSource `json:"sources"`
	Tracks  []DecryptedTrack  `json:"tracks"`
}

func DecryptStream(embedLink string, client *http.Client) (string, []string, error) {
	req, _ := http.NewRequest("GET", DECODER, nil)
	q := req.URL.Query()
	q.Add("url", embedLink)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", "curl/8.16.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("decoder returned status %d", resp.StatusCode)
	}

	var data DecryptResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", nil, err
	}

	var videoLink string
	for _, source := range data.Sources {
		if strings.Contains(source.File, ".m3u8") {
			videoLink = source.File
			break
		}
	}

	if videoLink == "" {
		return "", nil, fmt.Errorf("no m3u8 source found")
	}

	var subs []string
	for _, track := range data.Tracks {
		if track.Kind == "captions" || track.Kind == "subtitles" {
			if strings.Contains(strings.ToLower(track.Label), "english") {
				subs = append(subs, track.File)
			}
		}
	}

	return videoLink, subs, nil
}
