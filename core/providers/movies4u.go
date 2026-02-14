package providers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/demonkingswarn/luffy/core"
)

const (
	MOVIES4U_BASE_URL = "https://movies4u.am"
)

type Movies4u struct {
	Client *http.Client
}

func NewMovies4u(client *http.Client) *Movies4u {
	return &Movies4u{Client: client}
}

func (m *Movies4u) newRequest(method, url string) (*http.Request, error) {
	req, err := core.NewRequest(method, url)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", MOVIES4U_BASE_URL+"/")
	return req, nil
}

func (m *Movies4u) Search(query string) ([]core.SearchResult, error) {
	searchURL := fmt.Sprintf("%s/?s=%s", MOVIES4U_BASE_URL, strings.ReplaceAll(query, " ", "+"))
	req, _ := m.newRequest("GET", searchURL)

	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []core.SearchResult

	doc.Find("article.entry-card").Each(func(i int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Find("h2.entry-title a").Text())
		href := sel.Find("h2.entry-title a").AttrOr("href", "")
		poster := sel.Find("img.wp-post-image").AttrOr("src", "")
		if poster == "" {
			poster = sel.Find("img.wp-post-image").AttrOr("data-src", "")
		}

		if href != "" {
			results = append(results, core.SearchResult{
				Title:  title,
				URL:    href,
				Type:   core.Movie,
				Poster: poster,
			})
		}
	})

	if len(results) == 0 {
		return nil, errors.New("no results found")
	}

	return results, nil
}

func (m *Movies4u) GetMediaID(url string) (string, error) {
	return url, nil
}

func (m *Movies4u) GetSeasons(mediaID string) ([]core.Season, error) {
	return nil, nil
}

func (m *Movies4u) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if isSeason {
		return nil, nil
	}

	req, _ := m.newRequest("GET", id)
	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var episodes []core.Episode

	var bestLink string
	var bestQuality int // 0: none, 1: 480p, 2: 720p, 3: 1080p

	doc.Find("h5").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())

		var quality int
		if strings.Contains(text, "1080p") {
			quality = 3
		} else if strings.Contains(text, "720p") {
			quality = 2
		} else if strings.Contains(text, "480p") {
			quality = 1
		}

		if quality > 0 {
			nextP := sel.NextFiltered("p")
			link := nextP.Find("a").AttrOr("href", "")

			if link != "" && strings.Contains(link, "nexdrive.top") {
				if quality > bestQuality {
					bestQuality = quality
					bestLink = link
				}
			}
		}
	})

	if bestLink == "" {
		return nil, errors.New("no download links found")
	}

	episodes = append(episodes, core.Episode{
		ID:   bestLink,
		Name: "Full Movie",
	})

	return episodes, nil
}

func (m *Movies4u) GetServers(episodeID string) ([]core.Server, error) {
	return nil, nil
}

func (m *Movies4u) GetLink(serverID string) (string, error) {
	req, _ := m.newRequest("GET", serverID)
	resp, err := m.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	vCloud := doc.Find("a[href*='vcloud.zip']").AttrOr("href", "")
	if vCloud != "" {
		return m.handleVCloud(vCloud)
	}

	fastDl := doc.Find("a[href*='fastdl.zip']").AttrOr("href", "")
	if fastDl != "" {
		return m.resolveFinalLink(fastDl)
	}

	return "", errors.New("playable link not found on nexdrive page")
}

func (m *Movies4u) resolveFinalLink(url string) (string, error) {
	req, err := m.newRequest("GET", url)
	if err != nil {
		return "", err
	}

	resp, err := m.Client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	finalURL := resp.Request.URL.String()

	if strings.Contains(finalURL, "dl.php") && strings.Contains(finalURL, "link=") {
		q := resp.Request.URL.Query()
		return q.Get("link"), nil
	}

	return finalURL, nil
}

func (m *Movies4u) handleVCloud(url string) (string, error) {
	req, _ := m.newRequest("GET", url)
	resp, err := m.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	bodyString := string(bodyBytes)

	re := regexp.MustCompile(`var\s+url\s*=\s*'([^']*(?:hubcloud\.php)[^']*)'`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 2 {
		return "", errors.New("hubcloud link not found in vcloud page")
	}
	hubCloudURL := matches[1]

	return m.resolveHubCloudDownload(hubCloudURL)
}

func (m *Movies4u) resolveHubCloudDownload(url string) (string, error) {
	req, _ := m.newRequest("GET", url)
	resp, err := m.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	hubDoc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	var downloadLink string
	hubDoc.Find("a").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		text := strings.ToLower(sel.Text())
		if strings.Contains(text, "download") {
			href := sel.AttrOr("href", "")
			if href != "" && strings.HasPrefix(href, "http") {
				if !strings.Contains(href, "how-to") {
					downloadLink = href
					return false
				}
			}
		}
		return true
	})

	if downloadLink == "" {
		return "", errors.New("no download link found on final hubcloud page")
	}

	return m.resolveFinalLink(downloadLink)
}
