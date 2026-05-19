package providers

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/demonkingswarn/luffy/core"
)

const (
	ALLANIME_BASE_URL = "https://allanime.day"
	ALLANIME_API_URL  = "https://api.allanime.day/api"
	ALLANIME_REFERER  = "https://allmanga.to"
	ALLANIME_AGENT    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
)

type AllAnime struct {
	Client *http.Client
	Mode   string // "sub" or "dub"
}

func NewAllAnime(client *http.Client) *AllAnime {
	return &AllAnime{Client: client, Mode: "sub"}
}

func NewAllAnimeDub(client *http.Client) *AllAnime {
	return &AllAnime{Client: client, Mode: "dub"}
}

func NewAnime(client *http.Client) *AllAnime {
	return NewAllAnime(client)
}

func NewAnimeDub(client *http.Client) *AllAnime {
	return NewAllAnimeDub(client)
}

// decodeAllAnimeURL decodes the obfuscated source URLs returned by AllAnime.
// This is a direct port of ani-cli's provider_init hex substitution cipher.
func decodeAllAnimeURL(encoded string) string {
	hexMap := map[string]string{
		"79": "A", "7a": "B", "7b": "C", "7c": "D", "7d": "E",
		"7e": "F", "7f": "G", "70": "H", "71": "I", "72": "J",
		"73": "K", "74": "L", "75": "M", "76": "N", "77": "O",
		"68": "P", "69": "Q", "6a": "R", "6b": "S", "6c": "T",
		"6d": "U", "6e": "V", "6f": "W", "60": "X", "61": "Y",
		"62": "Z", "59": "a", "5a": "b", "5b": "c", "5c": "d",
		"5d": "e", "5e": "f", "5f": "g", "50": "h", "51": "i",
		"52": "j", "53": "k", "54": "l", "55": "m", "56": "n",
		"57": "o", "48": "p", "49": "q", "4a": "r", "4b": "s",
		"4c": "t", "4d": "u", "4e": "v", "4f": "w", "40": "x",
		"41": "y", "42": "z", "08": "0", "09": "1", "0a": "2",
		"0b": "3", "0c": "4", "0d": "5", "0e": "6", "0f": "7",
		"00": "8", "01": "9", "15": "-", "16": ".", "67": "_",
		"46": "~", "02": ":", "17": "/", "07": "?", "1b": "#",
		"63": "[", "65": "]", "78": "@", "19": "!", "1c": "$",
		"1e": "&", "10": "(", "11": ")", "12": "*", "13": "+",
		"14": ",", "03": ";", "05": "=", "1d": "%",
	}

	var result strings.Builder
	for i := 0; i < len(encoded)-1; i += 2 {
		pair := encoded[i : i+2]
		if decoded, ok := hexMap[pair]; ok {
			result.WriteString(decoded)
		} else {
			result.WriteString(pair)
		}
	}
	decoded := result.String()
	// ani-cli appends .json to /clock paths
	decoded = strings.ReplaceAll(decoded, "/clock", "/clock.json")
	return decoded
}

// allAnimeGQL makes a GraphQL POST request to the AllAnime API.
func (a *AllAnime) allAnimeGQL(query string, variables map[string]interface{}) ([]byte, error) {
	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gql payload: %w", err)
	}

	req, err := http.NewRequest("POST", ALLANIME_API_URL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", ALLANIME_REFERER)
	req.Header.Set("User-Agent", ALLANIME_AGENT)

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("allanime api request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read allanime response: %w", err)
	}
	return body, nil
}

// Search searches AllAnime for anime matching the query.
func (a *AllAnime) Search(query string) ([]core.SearchResult, error) {
	gql := `query( $search: SearchInput $limit: Int $page: Int $translationType: VaildTranslationTypeEnumType $countryOrigin: VaildCountryOriginEnumType ) { shows( search: $search limit: $limit page: $page translationType: $translationType countryOrigin: $countryOrigin ) { edges { _id name availableEpisodes __typename } }}`

	variables := map[string]interface{}{
		"search": map[string]interface{}{
			"allowAdult":   false,
			"allowUnknown": false,
			"query":        query,
		},
		"limit":           40,
		"page":            1,
		"translationType": a.Mode,
		"countryOrigin":   "ALL",
	}

	body, err := a.allAnimeGQL(gql, variables)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			Shows struct {
				Edges []struct {
					ID                string                 `json:"_id"`
					Name              string                 `json:"name"`
					AvailableEpisodes map[string]json.Number `json:"availableEpisodes"`
					Typename          string                 `json:"__typename"`
				} `json:"edges"`
			} `json:"shows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	var results []core.SearchResult
	for _, edge := range resp.Data.Shows.Edges {
		epCount := ""
		if ep, ok := edge.AvailableEpisodes[a.Mode]; ok {
			epCount = ep.String()
		}

		title := edge.Name
		if epCount != "" {
			title = fmt.Sprintf("%s (%s episodes)", edge.Name, epCount)
		}

		mediaType := core.Series
		if epCount == "1" {
			mediaType = core.Movie
		}

		results = append(results, core.SearchResult{
			Title: title,
			URL:   edge.ID,
			Type:  mediaType,
		})
	}

	if len(results) == 0 {
		return nil, errors.New("no results")
	}

	return results, nil
}

// GetMediaID for AllAnime, the "URL" is already the show ID.
func (a *AllAnime) GetMediaID(url string) (string, error) {
	if url == "" {
		return "", errors.New("empty show ID")
	}
	return url, nil
}

// GetSeasons returns a single virtual season for the anime.
func (a *AllAnime) GetSeasons(mediaID string) ([]core.Season, error) {
	gql := `query ($showId: String!) { show( _id: $showId ) { _id availableEpisodesDetail }}`

	variables := map[string]interface{}{
		"showId": mediaID,
	}

	body, err := a.allAnimeGQL(gql, variables)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			Show struct {
				AvailableEpisodesDetail map[string][]json.Number `json:"availableEpisodesDetail"`
			} `json:"show"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse episodes detail: %w", err)
	}

	episodes := resp.Data.Show.AvailableEpisodesDetail[a.Mode]
	if len(episodes) == 0 {
		return nil, nil
	}

	return []core.Season{
		{
			ID:   mediaID,
			Name: fmt.Sprintf("Season 1 (%d episodes)", len(episodes)),
		},
	}, nil
}

// GetEpisodes returns the list of episodes for the given show.
func (a *AllAnime) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	gql := `query ($showId: String!) { show( _id: $showId ) { _id availableEpisodesDetail }}`

	variables := map[string]interface{}{
		"showId": id,
	}

	body, err := a.allAnimeGQL(gql, variables)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			Show struct {
				AvailableEpisodesDetail map[string][]json.Number `json:"availableEpisodesDetail"`
			} `json:"show"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse episodes detail: %w", err)
	}

	epNumbers := resp.Data.Show.AvailableEpisodesDetail[a.Mode]
	if len(epNumbers) == 0 {
		return nil, errors.New("no episodes found")
	}

	sort.Slice(epNumbers, func(i, j int) bool {
		fi, _ := epNumbers[i].Float64()
		fj, _ := epNumbers[j].Float64()
		return fi < fj
	})

	var episodes []core.Episode
	for _, epNum := range epNumbers {
		epStr := epNum.String()
		episodes = append(episodes, core.Episode{
			ID:   id + "|" + epStr,
			Name: "Episode " + epStr,
		})
	}

	return episodes, nil
}

// knownSourceNames are the AllAnime source names that decode to API paths
// on allanime.day, matching ani-cli's generate_link() provider list.
var knownSourceNames = []string{"Default", "Yt-mp4", "S-mp4", "Luf-Mp4", "Fm-mp4"}

// GetServers resolves episode sources into playable stream URLs.
// Unlike other providers, AllAnime's GetServers does the full resolution:
//  1. Fetches source URLs from GraphQL (obfuscated hex-encoded)
//  2. Decodes them using the hex cipher
//  3. For API paths (starting with /), fetches https://allanime.day{path}
//     and extracts actual m3u8/mp4 stream links from the JSON response
//  4. Returns servers whose IDs are already playable stream URLs
//
// This matches ani-cli's generate_link() + get_links() flow.
func (a *AllAnime) GetServers(episodeID string) ([]core.Server, error) {
	parts := strings.SplitN(episodeID, "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid episode ID format: %s", episodeID)
	}
	showID := parts[0]
	epNo := parts[1]

	sourceURLs, err := a.getEpisodeSourceURLs(showID, epNo)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	serverCh := make(chan []core.Server, len(sourceURLs))
	for _, src := range sourceURLs {
		src := src
		wg.Add(1)
		go func() {
			defer wg.Done()
			serverCh <- a.resolveSourceServers(src)
		}()
	}
	wg.Wait()
	close(serverCh)

	var servers []core.Server
	for resolved := range serverCh {
		servers = append(servers, resolved...)
	}
	sort.SliceStable(servers, func(i, j int) bool { return qualityScore(servers[i].Name) > qualityScore(servers[j].Name) })

	if len(servers) == 0 {
		return nil, errors.New("no servers found for this episode")
	}

	return servers, nil
}

func (a *AllAnime) resolveSourceServers(src sourceURL) []core.Server {
	var servers []core.Server

	// Direct stream URLs (fast4speed, wixmp, etc.) — add as-is.
	if isDirectStreamURL(src.decodedURL) {
		servers = append(servers, core.Server{
			ID:   src.decodedURL,
			Name: src.sourceName,
		})
		return servers
	}

	// Only process API paths (starting with /) from known source names.
	if !strings.HasPrefix(src.decodedURL, "/") {
		return nil
	}
	isKnown := false
	for _, name := range knownSourceNames {
		if src.sourceName == name {
			isKnown = true
			break
		}
	}
	if !isKnown {
		return nil
	}

	// Fetch the allanime.day API endpoint to get actual stream links.
	apiURL := "https://allanime.day" + src.decodedURL
	var links []streamLink
	var fetchErr error
	if src.sourceName == "Fm-mp4" {
		links, fetchErr = a.fetchFilemoonLinks(apiURL)
	} else {
		links, fetchErr = a.fetchStreamLinks(apiURL)
	}
	if fetchErr != nil || len(links) == 0 {
		return nil
	}
	sort.SliceStable(links, func(i, j int) bool { return qualityScore(links[i].quality) > qualityScore(links[j].quality) })

	for _, link := range links {
		name := src.sourceName
		if link.quality != "" {
			name = fmt.Sprintf("%s (%s)", src.sourceName, link.quality)
		}
		servers = append(servers, core.Server{
			ID:   decorateStreamURL(link),
			Name: name,
		})
	}
	return servers
}

type sourceURL struct {
	sourceName string
	decodedURL string
}

// getEpisodeSourceURLs fetches and decodes the embed URLs for an episode.
func (a *AllAnime) getEpisodeSourceURLs(showID, epNo string) ([]sourceURL, error) {
	gql := `query ($showId: String!, $translationType: VaildTranslationTypeEnumType!, $episodeString: String!) { episode( showId: $showId translationType: $translationType episodeString: $episodeString ) { episodeString sourceUrls }}`

	variables := map[string]interface{}{
		"showId":          showID,
		"translationType": a.Mode,
		"episodeString":   epNo,
	}

	body, err := a.fetchEpisodeSourcesBody(gql, variables)
	if err != nil {
		return nil, err
	}
	if blob := extractToBeParsedBlob(body); blob != "" {
		return decodeToBeParsed(blob)
	}
	return parseEpisodeSources(body)
}

func (a *AllAnime) fetchEpisodeSourcesBody(gql string, variables map[string]interface{}) ([]byte, error) {
	queryHash := "d405d0edd690624b66baba3068e0edc3ac90f1597d898a1ec8db4e5c43c00fec"
	vars, _ := json.Marshal(variables)
	ext, _ := json.Marshal(map[string]interface{}{
		"persistedQuery": map[string]interface{}{"version": 1, "sha256Hash": queryHash},
	})
	apiURL := ALLANIME_API_URL + "?variables=" + url.QueryEscape(string(vars)) + "&extensions=" + url.QueryEscape(string(ext))
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err == nil {
		req.Header.Set("Referer", "https://youtu-chan.com")
		req.Header.Set("Origin", "https://youtu-chan.com")
		req.Header.Set("User-Agent", ALLANIME_AGENT)
		if resp, doErr := a.Client.Do(req); doErr == nil {
			defer resp.Body.Close()
			if body, readErr := io.ReadAll(resp.Body); readErr == nil && len(body) > 0 && bytes.Contains(body, []byte("tobeparsed")) {
				return body, nil
			}
		}
	}
	return a.allAnimeGQL(gql, variables)
}

func extractToBeParsedBlob(body []byte) string {
	var resp struct {
		Data struct {
			Episode struct {
				ToBeParsed string `json:"tobeparsed"`
			} `json:"episode"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &resp) == nil && resp.Data.Episode.ToBeParsed != "" {
		return resp.Data.Episode.ToBeParsed
	}
	re := regexp.MustCompile(`"tobeparsed"\s*:\s*"([^"]+)"`)
	if m := re.FindSubmatch(body); len(m) == 2 {
		return string(m[1])
	}
	return ""
}

func decodeToBeParsed(blob string) ([]sourceURL, error) {
	data, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted episode sources: %w", err)
	}
	if len(data) < 30 {
		return nil, errors.New("encrypted episode source payload is too short")
	}
	iv := make([]byte, aes.BlockSize)
	copy(iv, data[1:13])
	iv[15] = 2
	ctEnd := len(data) - 16
	if ctEnd <= 13 {
		return nil, errors.New("encrypted episode source ciphertext is empty")
	}
	key := sha256.Sum256([]byte("Xot36i3lK3:v1"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	plain := make([]byte, ctEnd-13)
	cipher.NewCTR(block, iv).XORKeyStream(plain, data[13:ctEnd])
	return parseEpisodeSources(plain)
}

func parseEpisodeSources(body []byte) ([]sourceURL, error) {

	var resp struct {
		Data struct {
			Episode struct {
				EpisodeString string `json:"episodeString"`
				SourceUrls    []struct {
					SourceURL  string `json:"sourceUrl"`
					SourceName string `json:"sourceName"`
				} `json:"sourceUrls"`
			} `json:"episode"`
		} `json:"data"`
	}

	var sources []sourceURL
	if err := json.Unmarshal(body, &resp); err == nil {
		for _, src := range resp.Data.Episode.SourceUrls {
			if parsed := normalizeSourceURL(src.SourceName, src.SourceURL); parsed.decodedURL != "" {
				sources = append(sources, parsed)
			}
		}
		if len(sources) > 0 {
			return sources, nil
		}
	}

	text := strings.ReplaceAll(string(body), `\u002F`, "/")
	text = strings.ReplaceAll(text, `\/`, "/")
	re := regexp.MustCompile(`"sourceUrl"\s*:\s*"--([^"]+)"[^{}]*"sourceName"\s*:\s*"([^"]+)"`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if parsed := normalizeSourceURL(m[2], m[1]); parsed.decodedURL != "" {
			sources = append(sources, parsed)
		}
	}
	if len(sources) == 0 {
		return nil, errors.New("no episode sources found")
	}

	return sources, nil
}

func normalizeSourceURL(sourceName, raw string) sourceURL {
	if strings.HasPrefix(raw, "--") {
		raw = raw[2:]
	}
	decoded := decodeAllAnimeURL(raw)
	if decoded == "" {
		return sourceURL{}
	}
	return sourceURL{sourceName: sourceName, decodedURL: decoded}
}

// GetLink for AllAnime simply returns the server ID, since GetServers
// already resolved API paths to actual playable stream URLs.
func (a *AllAnime) GetLink(serverID string) (string, error) {
	if serverID == "" {
		return "", errors.New("empty server ID")
	}
	return serverID, nil
}

type streamLink struct {
	quality   string
	url       string
	referer   string
	subtitles []string
}

// fetchStreamLinks fetches actual stream URLs from an AllAnime API endpoint.
// This is equivalent to ani-cli's get_links() function.
// Uses both structured JSON parsing and regex-based extraction for robustness.
func (a *AllAnime) fetchStreamLinks(fetchURL string) ([]streamLink, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", ALLANIME_REFERER)
	req.Header.Set("User-Agent", ALLANIME_AGENT)

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stream links: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	bodyStr := string(body)
	// Unescape unicode forward slashes that AllAnime uses
	bodyStr = strings.ReplaceAll(bodyStr, `\u002F`, `/`)
	bodyStr = strings.ReplaceAll(bodyStr, `\/`, `/`)

	var links []streamLink

	// Method 1: Structured JSON — {"links":[{"link":"...","resolutionStr":"..."},...]}
	var linkResp struct {
		Links []struct {
			Link          string `json:"link"`
			ResolutionStr string `json:"resolutionStr"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(bodyStr), &linkResp); err == nil {
		for _, l := range linkResp.Links {
			if l.Link != "" {
				links = append(links, streamLink{quality: l.ResolutionStr, url: l.Link})
			}
		}
		if wixmp := expandWixmpLinks(links); len(wixmp) > 0 {
			return wixmp, nil
		}
		if len(links) > 0 && !strings.Contains(bodyStr, "master.m3u8") {
			return links, nil
		}
	}

	// Method 2: Regex extraction matching ani-cli's sed patterns.
	// Pattern: "link":"<URL>"..."resolutionStr":"<RES>"
	linkRe := regexp.MustCompile(`"link"\s*:\s*"([^"]+)"`)
	resRe := regexp.MustCompile(`"resolutionStr"\s*:\s*"([^"]*)"`)

	// Split on },{ to handle array elements like ani-cli does
	chunks := strings.Split(bodyStr, "},{")
	for _, chunk := range chunks {
		linkMatch := linkRe.FindStringSubmatch(chunk)
		if len(linkMatch) < 2 || linkMatch[1] == "" {
			continue
		}
		quality := ""
		resMatch := resRe.FindStringSubmatch(chunk)
		if len(resMatch) >= 2 {
			quality = resMatch[1]
		}
		links = append(links, streamLink{quality: quality, url: linkMatch[1]})
	}
	if wixmp := expandWixmpLinks(links); len(wixmp) > 0 {
		return wixmp, nil
	}

	if strings.Contains(bodyStr, "master.m3u8") {
		master := firstMasterURL(links, bodyStr)
		if master != "" {
			m3u8Referer := extractJSONField(bodyStr, "Referer")
			if m3u8Referer == "" {
				m3u8Referer = ALLANIME_REFERER
			}
			variants, err := a.fetchM3U8Variants(master, m3u8Referer)
			if err == nil && len(variants) > 0 {
				sub := extractSubtitleURL(bodyStr)
				for i := range variants {
					variants[i].referer = m3u8Referer
					if sub != "" {
						variants[i].subtitles = []string{sub}
					}
				}
				return variants, nil
			}
		}
	}

	// Pattern: HLS URLs with hardsub_lang
	hlsRe := regexp.MustCompile(`"hls"[^}]*"url"\s*:\s*"([^"]+)"[^}]*"hardsub_lang"\s*:\s*"en-US"`)
	hlsMatches := hlsRe.FindAllStringSubmatch(bodyStr, -1)
	for _, m := range hlsMatches {
		if len(m) >= 2 && m[1] != "" {
			isDup := false
			for _, l := range links {
				if l.url == m[1] {
					isDup = true
					break
				}
			}
			if !isDup {
				links = append(links, streamLink{quality: "hls", url: m[1]})
			}
		}
	}
	if strings.Contains(fetchURL, "tools.fast4speed.rsvp") {
		links = append(links, streamLink{quality: "Yt", url: fetchURL, referer: ALLANIME_REFERER})
	}

	if len(links) == 0 {
		return nil, fmt.Errorf("no stream links found in API response")
	}

	return links, nil
}

func expandWixmpLinks(in []streamLink) []streamLink {
	var out []streamLink
	re := regexp.MustCompile(`.*,([^/]*),/mp4`)
	for _, link := range in {
		if !strings.Contains(link.url, "repackager.wixmp.com") {
			continue
		}
		extract := strings.SplitN(link.url, "repackager.wixmp.com/", 2)
		if len(extract) != 2 {
			continue
		}
		base := strings.Split(extract[1], ".urlset")[0]
		m := re.FindStringSubmatch(link.url)
		if len(m) != 2 {
			out = append(out, streamLink{quality: link.quality, url: base})
			continue
		}
		for _, q := range strings.Split(m[1], ",") {
			if q == "" {
				continue
			}
			variant := regexp.MustCompile(`,[^/]*`).ReplaceAllString(base, q)
			out = append(out, streamLink{quality: q, url: variant})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return qualityScore(out[i].quality) > qualityScore(out[j].quality) })
	return out
}

func firstMasterURL(links []streamLink, body string) string {
	for _, link := range links {
		if strings.Contains(link.url, "master.m3u8") {
			return link.url
		}
	}
	re := regexp.MustCompile(`https?://[^"\\]+master\.m3u8[^"\\]*`)
	return re.FindString(body)
}

func (a *AllAnime) fetchM3U8Variants(masterURL, referer string) ([]streamLink, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", masterURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", ALLANIME_AGENT)
	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(body), "EXTM3U") {
		return nil, errors.New("not an m3u8 playlist")
	}
	base, _ := url.Parse(masterURL)
	lines := strings.Split(string(body), "\n")
	var out []streamLink
	quality := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXT-X-STREAM") {
			quality = ""
			if idx := strings.Index(line, "x"); idx != -1 {
				rest := line[idx+1:]
				quality = strings.Split(rest, ",")[0] + "p"
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.Contains(line, "EXT-X-I-FRAME") {
			continue
		}
		resolved := line
		if u, err := url.Parse(line); err == nil && base != nil {
			resolved = base.ResolveReference(u).String()
		}
		out = append(out, streamLink{quality: quality, url: resolved})
		quality = ""
	}
	sort.SliceStable(out, func(i, j int) bool { return qualityScore(out[i].quality) > qualityScore(out[j].quality) })
	return out, nil
}

func extractJSONField(body, field string) string {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `"\s*:\s*"([^"]+)"`)
	if m := re.FindStringSubmatch(body); len(m) == 2 {
		return strings.ReplaceAll(m[1], `\u0026`, "&")
	}
	return ""
}

func extractSubtitleURL(body string) string {
	re := regexp.MustCompile(`"subtitles"\s*:\s*\[\s*\{[^\]]*"lang"\s*:\s*"en"[^\]]*"src"\s*:\s*"([^"]+)"`)
	if m := re.FindStringSubmatch(body); len(m) == 2 {
		return strings.ReplaceAll(m[1], `\u0026`, "&")
	}
	return ""
}

func (a *AllAnime) fetchFilemoonLinks(fetchURL string) ([]streamLink, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", ALLANIME_REFERER)
	req.Header.Set("User-Agent", ALLANIME_AGENT)
	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		IV       string   `json:"iv"`
		Payload  string   `json:"payload"`
		KeyParts []string `json:"key_parts"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.IV == "" || payload.Payload == "" || len(payload.KeyParts) < 2 {
		return nil, errors.New("invalid filemoon payload")
	}
	kp1, err := b64URLDecode(payload.KeyParts[0])
	if err != nil {
		return nil, err
	}
	kp2, err := b64URLDecode(payload.KeyParts[1])
	if err != nil {
		return nil, err
	}
	key := append(kp1, kp2...)
	ivRaw, err := b64URLDecode(payload.IV)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)
	copy(iv, ivRaw)
	iv[15] = 2
	ciphertext, err := b64URLDecode(payload.Payload)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) > 16 {
		ciphertext = ciphertext[:len(ciphertext)-16]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCTR(block, iv).XORKeyStream(plain, ciphertext)
	text := strings.ReplaceAll(string(plain), `\u0026`, "&")
	text = strings.ReplaceAll(text, `\u003D`, "=")
	re := regexp.MustCompile(`"url"\s*:\s*"([^"]+)"[^{}]*"height"\s*:\s*([0-9]+)|"height"\s*:\s*([0-9]+)[^{}]*"url"\s*:\s*"([^"]+)"`)
	var out []streamLink
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		quality, streamURL := m[2], m[1]
		if streamURL == "" {
			quality, streamURL = m[3], m[4]
		}
		if streamURL != "" {
			out = append(out, streamLink{quality: quality + "p", url: streamURL})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no filemoon streams found")
	}
	return out, nil
}

func b64URLDecode(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

func decorateStreamURL(link streamLink) string {
	stream := link.url
	if link.referer != "" {
		stream += "|referer=" + url.QueryEscape(link.referer)
	}
	if len(link.subtitles) > 0 {
		stream += "|subs=" + url.QueryEscape(strings.Join(link.subtitles, ","))
	}
	return stream
}

func qualityScore(q string) int {
	re := regexp.MustCompile(`[0-9]+`)
	if m := re.FindString(q); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			return n
		}
	}
	if strings.EqualFold(q, "hls") {
		return 1
	}
	return 0
}

// isDirectStreamURL checks if the URL points directly to a playable file.
func isDirectStreamURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, ".m3u8") ||
		strings.Contains(lower, ".mp4") ||
		strings.Contains(lower, ".mkv") ||
		strings.Contains(lower, "repackager.wixmp.com") ||
		strings.Contains(lower, "tools.fast4speed.rsvp")
}
