package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

func DecryptStream(embedLink string, client *http.Client) (string, []string, string, error) {
	if strings.Contains(embedLink, "vidsrc.xyz") ||
		strings.Contains(embedLink, "vidsrc.me") ||
		strings.Contains(embedLink, "vidsrc.to") ||
		strings.Contains(embedLink, "vidsrc.in") ||
		strings.Contains(embedLink, "vidsrc.pm") ||
		strings.Contains(embedLink, "vidsrc.net") {
		return DecryptVidsrc(embedLink, client)
	}

	if strings.Contains(embedLink, "vidlink.pro") {
		return DecryptVidlink(embedLink, client)
	}

	if strings.Contains(embedLink, "embed.su") {
		return DecryptEmbedSu(embedLink, client)
	}

	if strings.Contains(embedLink, "multiembed.mov") {
		return DecryptStreamWithDecoder(embedLink, client)
	}

	return DecryptStreamWithDecoder(embedLink, client)
}

func DecryptVidsrc(urlStr string, client *http.Client) (string, []string, string, error) {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	reCloud := regexp.MustCompile(`src="//cloudnestra\.com/rcp/([^"]+)"`)
	match := reCloud.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, "", fmt.Errorf("could not find cloudnestra iframe")
	}
	hash := string(match[1])
	cloudUrl := "https://cloudnestra.com/rcp/" + hash

	req, _ = http.NewRequest("GET", cloudUrl, nil)
	req.Header.Set("Referer", urlStr)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err = client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)

	rePro := regexp.MustCompile(`src:\s*'/prorcp/([^']+)'`)
	match = rePro.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, "", fmt.Errorf("could not find prorcp iframe")
	}
	proUrl := "https://cloudnestra.com/prorcp/" + string(match[1])

	req, _ = http.NewRequest("GET", proUrl, nil)
	req.Header.Set("Referer", "https://cloudnestra.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err = client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)

	reFile := regexp.MustCompile(`file:\s*"(https://[^"]+)"`)
	match = reFile.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, "", fmt.Errorf("could not find m3u8 file")
	}
	rawM3u8 := string(match[1])

	finalUrl := rawM3u8
	placeholders := []string{"{v1}", "{v2}", "{v3}", "{v4}"}
	for _, p := range placeholders {
		finalUrl = strings.ReplaceAll(finalUrl, p, "cloudnestra.com")
	}

	if idx := strings.Index(finalUrl, " or "); idx != -1 {
		finalUrl = finalUrl[:idx]
	}

	if !strings.HasSuffix(finalUrl, ".m3u8") {
		return "", nil, "", fmt.Errorf("extracted url is not m3u8: %s", finalUrl)
	}

	var subs []string
	parsedUrl, _ := url.Parse(urlStr)
	subUrl := fmt.Sprintf("%s://%s/ajax/embed/episode/%s/subtitles", parsedUrl.Scheme, parsedUrl.Host, hash)
	subReq, _ := http.NewRequest("GET", subUrl, nil)
	subReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	subReq.Header.Set("Referer", urlStr)
	subReq.Header.Set("X-Requested-With", "XMLHttpRequest")

	subResp, err := client.Do(subReq)
	if err == nil && subResp.StatusCode == 200 {
		var tracks []DecryptedTrack
		if err := json.NewDecoder(subResp.Body).Decode(&tracks); err == nil {
			for _, track := range tracks {
				label := strings.ToLower(track.Label)
				if strings.Contains(label, "english") || strings.Contains(label, " eng") || label == "eng" {
					subs = append(subs, track.File)
				}
			}
		}
		subResp.Body.Close()
	}

	return finalUrl, subs, "https://cloudnestra.com/", nil
}

func DecryptVidlink(urlStr string, client *http.Client) (string, []string, string, error) {
	re := regexp.MustCompile(`/(movie|tv)/([^/?#]+)`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) < 3 {
		return "", nil, "", fmt.Errorf("could not parse vidlink url")
	}

	tmdbID := matches[2]
	subUrl := fmt.Sprintf("https://vidlink.pro/api/subtitles/%s", tmdbID)

	req, _ := http.NewRequest("GET", subUrl, nil)
	resp, err := client.Do(req)

	var subs []string
	if err == nil && resp.StatusCode == 200 {
		var tracks []struct {
			URL   string `json:"url"`
			Label string `json:"label"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&tracks); err == nil {
			for _, t := range tracks {
				label := strings.ToLower(t.Label)
				if strings.Contains(label, "english") || strings.Contains(label, " eng") || label == "eng" {
					subs = append(subs, t.URL)
				}
			}
		}
		resp.Body.Close()
	}

	videoLink, _, referer, err := DecryptStreamWithDecoder(urlStr, client)
	return videoLink, subs, referer, err
}

func DecryptEmbedSu(urlStr string, client *http.Client) (string, []string, string, error) {
	videoLink, subs, referer, err := DecryptStreamWithDecoder(urlStr, client)
	return videoLink, subs, referer, err
}

func DecryptStreamWithDecoder(embedLink string, client *http.Client) (string, []string, string, error) {
	req, _ := http.NewRequest("GET", DECODER, nil)
	q := req.URL.Query()
	q.Add("url", embedLink)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", "curl/8.18.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", nil, "", fmt.Errorf("decoder returned status %d", resp.StatusCode)
	}

	var data DecryptResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", nil, "", err
	}

	var videoLink string
	for _, source := range data.Sources {
		if strings.Contains(source.File, ".m3u8") {
			videoLink = source.File
			break
		}
	}

	if videoLink == "" {
		return "", nil, "", fmt.Errorf("no m3u8 source found")
	}

	var subs []string
	for _, track := range data.Tracks {
		if track.Kind == "captions" || track.Kind == "subtitles" {
			label := strings.ToLower(track.Label)
			if strings.Contains(label, "english") || strings.Contains(label, " eng") || label == "eng" {
				subs = append(subs, track.File)
			}
		}
	}

	return videoLink, subs, "", nil
}
