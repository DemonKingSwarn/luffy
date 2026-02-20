package core

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Quality struct {
	URL        string
	Resolution string
	Bandwidth  int
	Height     int
	Label      string
}

func GetQualities(m3u8URL string, client *http.Client, referer string) ([]Quality, string, error) {
	req, err := http.NewRequest("GET", m3u8URL, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("failed to fetch m3u8: %d", resp.StatusCode)
	}

	baseURL, err := url.Parse(m3u8URL)
	if err != nil {
		return nil, "", err
	}

	scanner := bufio.NewScanner(resp.Body)

	var qualities []Quality
	var currentBandwidth int
	var currentResolution string
	var currentHeight int
	var isVariant bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			isVariant = true
			currentBandwidth = 0
			currentResolution = ""
			currentHeight = 0

			if strings.Contains(line, "BANDWIDTH=") {
				parts := strings.Split(line, "BANDWIDTH=")
				if len(parts) > 1 {
					val := strings.Split(parts[1], ",")[0]
					currentBandwidth, _ = strconv.Atoi(val)
				}
			}

			if strings.Contains(line, "RESOLUTION=") {
				parts := strings.Split(line, "RESOLUTION=")
				if len(parts) > 1 {
					val := strings.Split(parts[1], ",")[0]
					resParts := strings.Split(val, "x")
					if len(resParts) == 2 {
						currentResolution = val
						currentHeight, _ = strconv.Atoi(resParts[1])
					}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		if isVariant && line != "" {
			isVariant = false

			label := currentResolution
			if label == "" {
				label = formatBandwidth(currentBandwidth)
			}

			resolvedURL := line
			u, err := url.Parse(line)
			if err == nil {
				resolvedURL = baseURL.ResolveReference(u).String()
			}

			qualities = append(qualities, Quality{
				URL:        resolvedURL,
				Resolution: currentResolution,
				Bandwidth:  currentBandwidth,
				Height:     currentHeight,
				Label:      label,
			})
		}
	}

	if len(qualities) > 0 {
		return qualities, "", nil
	}

	return nil, m3u8URL, nil
}

func formatBandwidth(bandwidth int) string {
	if bandwidth >= 1000000 {
		return fmt.Sprintf("%.1f Mbps", float64(bandwidth)/1000000)
	}
	return fmt.Sprintf("%.0f Kbps", float64(bandwidth)/1000)
}

func GetBestQuality(qualities []Quality) Quality {
	if len(qualities) == 0 {
		return Quality{}
	}

	best := qualities[0]
	for _, q := range qualities[1:] {
		if q.Height > best.Height || (q.Height == best.Height && q.Bandwidth > best.Bandwidth) {
			best = q
		}
	}
	return best
}

func SelectQuality(qualities []Quality, selectBest bool) (string, error) {
	if len(qualities) == 0 {
		return "", fmt.Errorf("no qualities available")
	}

	if selectBest || len(qualities) == 1 {
		best := GetBestQuality(qualities)
		return best.URL, nil
	}

	var labels []string
	for _, q := range qualities {
		labels = append(labels, q.Label)
	}

	idx := Select("Quality:", labels)
	return qualities[idx].URL, nil
}
