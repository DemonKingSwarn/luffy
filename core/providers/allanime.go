package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

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
var knownSourceNames = []string{"Default", "Yt-mp4", "S-mp4", "Luf-Mp4"}

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

	var servers []core.Server

	for _, src := range sourceURLs {
		// Direct stream URLs (fast4speed, wixmp, etc.) — add as-is.
		if isDirectStreamURL(src.decodedURL) {
			servers = append(servers, core.Server{
				ID:   src.decodedURL,
				Name: src.sourceName,
			})
			continue
		}

		// Only process API paths (starting with /) from known source names.
		if !strings.HasPrefix(src.decodedURL, "/") {
			continue
		}
		isKnown := false
		for _, name := range knownSourceNames {
			if src.sourceName == name {
				isKnown = true
				break
			}
		}
		if !isKnown {
			continue
		}

		// Fetch the allanime.day API endpoint to get actual stream links.
		apiURL := "https://allanime.day" + src.decodedURL
		links, fetchErr := a.fetchStreamLinks(apiURL)
		if fetchErr != nil || len(links) == 0 {
			continue
		}

		for _, link := range links {
			name := src.sourceName
			if link.quality != "" {
				name = fmt.Sprintf("%s (%s)", src.sourceName, link.quality)
			}
			servers = append(servers, core.Server{
				ID:   link.url,
				Name: name,
			})
		}
	}

	if len(servers) == 0 {
		return nil, errors.New("no servers found for this episode")
	}

	return servers, nil
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

	body, err := a.allAnimeGQL(gql, variables)
	if err != nil {
		return nil, err
	}

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
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse episode sources: %w", err)
	}

	var sources []sourceURL
	for _, src := range resp.Data.Episode.SourceUrls {
		raw := src.SourceURL
		if strings.HasPrefix(raw, "--") {
			raw = raw[2:]
		}
		decoded := decodeAllAnimeURL(raw)
		if decoded == "" {
			continue
		}
		sources = append(sources, sourceURL{
			sourceName: src.SourceName,
			decodedURL: decoded,
		})
	}

	return sources, nil
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
	quality string
	url     string
}

// fetchStreamLinks fetches actual stream URLs from an AllAnime API endpoint.
// This is equivalent to ani-cli's get_links() function.
// Uses both structured JSON parsing and regex-based extraction for robustness.
func (a *AllAnime) fetchStreamLinks(fetchURL string) ([]streamLink, error) {
	req, err := http.NewRequest("GET", fetchURL, nil)
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
		if len(links) > 0 {
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

	// Pattern: HLS URLs with hardsub_lang
	hlsRe := regexp.MustCompile(`"hls"[^}]*"url"\s*:\s*"([^"]+)"`)
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

	if len(links) == 0 {
		return nil, fmt.Errorf("no stream links found in API response")
	}

	return links, nil
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
