package core

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func GetBestQualityM3U8(m3u8URL string, client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", m3u8URL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch m3u8: %d", resp.StatusCode)
	}

	baseURL, err := url.Parse(m3u8URL)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(resp.Body)

	var bestURL string
	var maxBandwidth int
	var maxResolution int

	var currentBandwidth int
	var currentResolution int
	var isVariant bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			isVariant = true
			currentBandwidth = 0
			currentResolution = 0

			// Parse BANDWIDTH
			if strings.Contains(line, "BANDWIDTH=") {
				parts := strings.Split(line, "BANDWIDTH=")
				if len(parts) > 1 {
					val := strings.Split(parts[1], ",")[0]
					currentBandwidth, _ = strconv.Atoi(val)
				}
			}

			// Parse RESOLUTION
			if strings.Contains(line, "RESOLUTION=") {
				parts := strings.Split(line, "RESOLUTION=")
				if len(parts) > 1 {
					val := strings.Split(parts[1], ",")[0]
					resParts := strings.Split(val, "x")
					if len(resParts) == 2 {
						w, _ := strconv.Atoi(resParts[0])
						h, _ := strconv.Atoi(resParts[1])
						currentResolution = w * h
					}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		if isVariant && line != "" {
			// This is the URL line for the variant
			isVariant = false

			// Simple heuristic: resolution > bandwidth
			isBetter := false
			if currentResolution > maxResolution {
				isBetter = true
			} else if currentResolution == maxResolution {
				if currentBandwidth > maxBandwidth {
					isBetter = true
				}
			}

			if isBetter || bestURL == "" {
				maxResolution = currentResolution
				maxBandwidth = currentBandwidth

				// Resolve relative URL
				u, err := url.Parse(line)
				if err == nil {
					bestURL = baseURL.ResolveReference(u).String()
				} else {
					bestURL = line // Fallback
				}
			}
		}
	}

	if bestURL != "" {
		return bestURL, nil
	}

	// If no variants found, it might be a simple media playlist or parsing failed
	// Return original URL
	return m3u8URL, nil
}
