package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/demonkingswarn/luffy/core"
)

const (
	// StreamoBaseURL is the site origin used for Search result URLs, metadata,
	// and the Referer/Origin values required by Streamo's streaming proxy.
	StreamoBaseURL = "https://streamo.st"

	stMaxConcurrency = 4
	stHTTPTimeout    = 15 * time.Second
	stResolveTimeout = 20 * time.Second
	stRaceTimeout    = 60 * time.Second
	stPlaylistLimit  = 4 << 20
	stTokenLimit     = 8 << 20
)

// Streamo is a browser-free Streamo provider: metadata comes from TMDB and
// stream resolution asks Streamo for a guest token, then races every provider
// it advertises and returns the first stream that validates.
type Streamo struct {
	Client *http.Client

	mu sync.Mutex
}

// stRequest is the resolved identity carried through the Provider interface:
//
//	movie:  movie|<tmdb>
//	series: series|<tmdb>                          (show)
//	series: series|<tmdb>|<season>                 (season)
//	series: series|<tmdb>|<season>|<episode>       (episode)
type stRequest struct {
	kind    string // movie | series
	tmdb    string
	season  string
	episode string
}

// stProvider is one upstream source advertised by Streamo's guest token. Ref
// is an opaque resolver handle, not a URL.
type stProvider struct {
	Name         string `json:"name"`
	Ref          string `json:"url"`
	Kind         string `json:"kind"`
	AllQualities []struct {
		Quality string `json:"quality"`
		Ref     string `json:"url"`
	} `json:"allQualities"`
}

func NewStreamo(client *http.Client) *Streamo {
	return &Streamo{Client: client}
}

// --- ID encoding -----------------------------------------------------------

func streamoID(st stRequest) string {
	parts := []string{st.kind, st.tmdb}
	if st.kind == "series" {
		// Omit empty trailing parts so show-level IDs stay "series|<tmdb>".
		if st.season != "" || st.episode != "" {
			parts = append(parts, st.season)
		}
		if st.episode != "" {
			parts = append(parts, st.episode)
		}
	}
	return strings.Join(parts, "|")
}

func parseStreamoID(id string) (st stRequest, err error) {
	parts := strings.Split(id, "|")
	if len(parts) < 2 {
		return st, fmt.Errorf("invalid streamo ID")
	}
	st.kind = parts[0]
	st.tmdb = parts[1]
	if st.tmdb == "" || !(st.kind == "movie" || st.kind == "series") {
		return st, fmt.Errorf("invalid streamo ID")
	}
	if st.kind == "series" && len(parts) > 2 {
		st.season = parts[2]
	}
	if st.kind == "series" && len(parts) > 3 {
		st.episode = parts[3]
	}
	return st, nil
}

// --- Provider interface -----------------------------------------------------

func (s *Streamo) Search(query string) ([]core.SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("include_adult", "false")
	params.Set("language", "en-US")
	params.Set("page", "1")
	params.Set("api_key", core.TMDB_API_KEY)

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/search/multi?%s", core.TMDB_BASE_URL, params.Encode()))
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []core.SearchResult
	for _, r := range data.Results {
		if r.MediaType != "movie" && r.MediaType != "tv" {
			continue
		}
		title := r.Title
		if title == "" {
			title = r.Name
		}
		year := r.ReleaseDate
		if year == "" {
			year = r.FirstAirDate
		}
		if len(year) > 4 {
			year = year[:4]
		}
		mediaType := core.Movie
		if r.MediaType == "tv" {
			mediaType = core.Series
		}
		poster := ""
		if r.PosterPath != "" {
			poster = core.TMDB_IMAGE_BASE_URL + r.PosterPath
		}
		results = append(results, core.SearchResult{
			Title:  title,
			URL:    fmt.Sprintf("%s/%s/%d", StreamoBaseURL, streamoPathFor(r.MediaType), r.ID),
			Type:   mediaType,
			Poster: poster,
			Year:   year,
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results")
	}
	return results, nil
}

// streamoPathFor maps a TMDB media type to Streamo's route segment.
func streamoPathFor(tmdbType string) string {
	if tmdbType == "tv" {
		return "series"
	}
	return "movies"
}

func (s *Streamo) GetMediaID(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[1] == "" {
		return "", fmt.Errorf("invalid streamo URL")
	}
	var kind string
	switch parts[0] {
	case "movies", "movie":
		kind = "movie"
	case "series", "tv":
		kind = "series"
	default:
		return "", fmt.Errorf("invalid streamo URL")
	}
	// Tolerate slug suffixes: /movies/1153576-street-fighter-2026
	tmdb := parts[1]
	if idx := strings.IndexAny(tmdb, "-"); idx != -1 {
		tmdb = tmdb[:idx]
	}
	if tmdb == "" {
		return "", fmt.Errorf("invalid streamo URL")
	}
	return streamoID(stRequest{kind: kind, tmdb: tmdb}), nil
}

func (s *Streamo) GetSeasons(mediaID string) ([]core.Season, error) {
	st, err := parseStreamoID(mediaID)
	if err != nil {
		return nil, err
	}
	if st.kind != "series" {
		return nil, nil
	}

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s?api_key=%s", core.TMDB_BASE_URL, st.tmdb, core.TMDB_API_KEY))
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbShowDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var seasons []core.Season
	for _, se := range data.Seasons {
		if se.SeasonNumber == 0 {
			continue
		}
		name := se.Name
		if name == "" {
			name = fmt.Sprintf("Season %d", se.SeasonNumber)
		}
		seasons = append(seasons, core.Season{
			ID:   streamoID(stRequest{kind: "series", tmdb: st.tmdb, season: strconv.Itoa(se.SeasonNumber)}),
			Name: name,
		})
	}
	return seasons, nil
}

func (s *Streamo) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if !isSeason {
		// Movie path: the episode list IS the server list.
		return []core.Episode{{ID: id, Name: "Streamo"}}, nil
	}

	st, err := parseStreamoID(id)
	if err != nil {
		return nil, err
	}
	if st.kind != "series" || st.season == "" {
		return nil, fmt.Errorf("invalid streamo season ID")
	}

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s/season/%s?api_key=%s", core.TMDB_BASE_URL, st.tmdb, st.season, core.TMDB_API_KEY))
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbSeasonDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var episodes []core.Episode
	for _, e := range data.Episodes {
		episodes = append(episodes, core.Episode{
			ID:   streamoID(stRequest{kind: "series", tmdb: st.tmdb, season: st.season, episode: strconv.Itoa(e.EpisodeNumber)}),
			Name: fmt.Sprintf("E%02d - %s", e.EpisodeNumber, e.Name),
		})
	}
	return episodes, nil
}

func (s *Streamo) GetServers(id string) ([]core.Server, error) {
	// Resolution automatically races every provider from the guest token and
	// returns the first validated stream, so a single auto server is exposed.
	return []core.Server{{ID: id, Name: "Streamo (Auto)"}}, nil
}

// --- Resolution -------------------------------------------------------------

// GetLink requests a guest token for the media, then races every advertised
// provider; the first stream that validates as HLS wins.
func (s *Streamo) GetLink(serverID string) (string, error) {
	st, err := parseStreamoID(serverID)
	if err != nil {
		return "", err
	}
	if st.kind == "series" && (st.season == "" || st.episode == "") {
		return "", fmt.Errorf("streamo: series resolution requires season and episode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), stRaceTimeout)
	defer cancel()

	token, providers, err := s.fetchToken(ctx, st)
	if err != nil {
		return "", err
	}
	if len(providers) == 0 {
		return "", fmt.Errorf("streamo: no providers available")
	}

	winner := make(chan string, 1)
	var errMu sync.Mutex
	var failures []string

	next := 0
	workers := stMaxConcurrency
	if workers > len(providers) {
		workers = len(providers)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var prov stProvider
				s.mu.Lock()
				if next < len(providers) {
					prov = providers[next]
					next++
				}
				s.mu.Unlock()
				if prov.Ref == "" && prov.Name == "" {
					return
				}
				stream, serr := s.tryProvider(ctx, token, prov)
				if serr != nil {
					errMu.Lock()
					failures = append(failures, fmt.Sprintf("%s: %v", prov.Name, serr))
					errMu.Unlock()
					continue
				}
				select {
				case winner <- stream:
					cancel() // first validated stream wins; stop the rest
				default:
					cancel()
				}
				return
			}
		}()
	}
	wg.Wait()

	select {
	case stream := <-winner:
		return stream, nil
	default:
		errMu.Lock()
		msg := strings.Join(failures, "; ")
		errMu.Unlock()
		if msg == "" {
			msg = "all attempts cancelled"
		}
		return "", fmt.Errorf("streamo: no playable stream: %s", msg)
	}
}

// fetchToken asks Streamo for a guest token plus the provider list. Type is
// "movie" or "tv"; season/episode are sent for series.
func (s *Streamo) fetchToken(ctx context.Context, st stRequest) (string, []stProvider, error) {
	tmdbID, err := strconv.Atoi(st.tmdb)
	if err != nil {
		return "", nil, fmt.Errorf("streamo: invalid tmdb id: %w", err)
	}
	mediaType := "movie"
	if st.kind == "series" {
		mediaType = "tv"
	}
	payload := map[string]any{"tmdbId": tmdbID, "type": mediaType}
	if st.kind == "series" {
		season, err := strconv.Atoi(st.season)
		if err != nil {
			return "", nil, fmt.Errorf("streamo: invalid season: %w", err)
		}
		episode, err := strconv.Atoi(st.episode)
		if err != nil {
			return "", nil, fmt.Errorf("streamo: invalid episode: %w", err)
		}
		payload["season"] = season
		payload["episode"] = episode
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, stHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, StreamoBaseURL+"/api/streaming/guest-token", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	stSetHeaders(req)

	resp, err := s.Client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("streamo: fetch token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("streamo: token status %d", resp.StatusCode)
	}

	var data struct {
		Success bool `json:"success"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
		Data struct {
			Token     string       `json:"token"`
			Providers []stProvider `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, stTokenLimit)).Decode(&data); err != nil {
		return "", nil, fmt.Errorf("streamo: invalid token response: %w", err)
	}
	if !data.Success || data.Data.Token == "" {
		msg := "unsuccessful"
		if data.Error != nil && data.Error.Message != "" {
			msg = data.Error.Message
		}
		return "", nil, fmt.Errorf("streamo: token request failed: %s", msg)
	}
	return data.Data.Token, data.Data.Providers, nil
}

// tryProvider resolves one provider's refs (default first, then per-quality)
// and returns the first stream that validates.
func (s *Streamo) tryProvider(parent context.Context, token string, prov stProvider) (string, error) {
	ctx, cancel := context.WithTimeout(parent, stResolveTimeout)
	defer cancel()

	refs := make([]string, 0, 1+len(prov.AllQualities))
	if prov.Ref != "" {
		refs = append(refs, prov.Ref)
	}
	for _, q := range prov.AllQualities {
		if q.Ref != "" {
			refs = append(refs, q.Ref)
		}
		if len(refs) >= 4 {
			break
		}
	}
	if len(refs) == 0 {
		return "", fmt.Errorf("no refs")
	}

	var lastErr error
	for _, ref := range refs {
		stream, err := s.resolve(ctx, token, ref)
		if err != nil {
			lastErr = err
			continue
		}
		if err := stValidateStream(ctx, s.Client, stream); err != nil {
			lastErr = err
			continue
		}
		return stream, nil
	}
	return "", lastErr
}

// resolve trades an opaque ref for a playable stream URL. The API may return
// a relative proxy path, which is prefixed with the site origin.
func (s *Streamo) resolve(ctx context.Context, token, ref string) (string, error) {
	params := url.Values{}
	params.Set("ref", ref)
	params.Set("stoken", token)

	reqCtx, cancel := context.WithTimeout(ctx, stResolveTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, StreamoBaseURL+"/api/streaming/resolve?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	stSetHeaders(req)

	resp, err := s.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resolve status %d", resp.StatusCode)
	}

	var data struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data); err != nil {
		return "", fmt.Errorf("invalid resolve response: %w", err)
	}
	if data.URL == "" {
		return "", fmt.Errorf("empty resolve response")
	}
	if strings.HasPrefix(data.URL, "/") {
		return StreamoBaseURL + data.URL, nil
	}
	return data.URL, nil
}

// --- Helpers ----------------------------------------------------------------

// stValidateStream proves a candidate is playable the way a player actually
// consumes it: the playlist must be HLS, and the first things an HLS player
// downloads — the encryption key (if any), the fMP4 init map (if any), and
// the first media segment — must fetch successfully as non-HTML media.
//
// Some upstreams (e.g. the movy.sx/wing source) answer unauthorized clients
// with a decoy playlist whose "segments" are image stubs (playlist_000.jpg
// ...) that 404. Checking only for #EXTM3U lets that garbage win the race and
// then die inside mpv seconds later, before it ever renders a frame.
func stValidateStream(ctx context.Context, client *http.Client, rawURL string) error {
	mediaURL, body, err := stFetchPlaylist(ctx, client, rawURL)
	if err != nil {
		return err
	}
	if !stIsHLS(body) {
		return fmt.Errorf("not an HLS playlist")
	}

	// Drill into the first variant (what a player would pick) until we reach
	// a media playlist; more than two levels of nesting is unsupported.
	for depth := 0; strings.Contains(body, "#EXT-X-STREAM-INF"); depth++ {
		if depth >= 2 {
			return fmt.Errorf("nested master playlist")
		}
		variant := stFirstURI(body)
		if variant == "" {
			return fmt.Errorf("master playlist has no variants")
		}
		mediaURL, body, err = stFetchPlaylist(ctx, client, stResolveRef(mediaURL, variant))
		if err != nil {
			return fmt.Errorf("variant: %w", err)
		}
		if !stIsHLS(body) {
			return fmt.Errorf("variant is not an HLS playlist")
		}
	}

	type probe struct{ role, uri string }
	var probes []probe
	if u, ok := stTagURI(body, "#EXT-X-KEY:"); ok {
		probes = append(probes, probe{"key", u})
	}
	if u, ok := stTagURI(body, "#EXT-X-MAP:"); ok {
		probes = append(probes, probe{"init", u})
	}
	first := stFirstURI(body)
	if first == "" {
		return fmt.Errorf("media playlist has no segments")
	}
	probes = append(probes, probe{"segment", first})

	errCh := make(chan error, len(probes))
	var wg sync.WaitGroup
	for _, p := range probes {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			if perr := stProbeMedia(ctx, client, stResolveRef(mediaURL, p.uri)); perr != nil {
				errCh <- fmt.Errorf("%s: %w", p.role, perr)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	if err, ok := <-errCh; ok {
		return err
	}
	return nil
}

// stFetchPlaylist downloads an HLS playlist (master or media) and returns the
// final URL (after redirects) plus its body.
func stFetchPlaylist(ctx context.Context, client *http.Client, rawURL string) (finalURL, body string, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, stHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	stSetHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, stPlaylistLimit))
	if err != nil {
		return "", "", err
	}
	final := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return final, string(b), nil
}

// stProbeMedia fetches the head of a player-critical resource (key, init map,
// segment) and rejects HTTP errors, HTML error pages, and image stubs.
func stProbeMedia(ctx context.Context, client *http.Client, rawURL string) error {
	reqCtx, cancel := context.WithTimeout(ctx, stHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	stSetHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	trimmed := strings.TrimLeft(string(head), " \t\r\n")
	if strings.HasPrefix(trimmed, "#EXTM3U") {
		return nil // a nested playlist is still playlist-shaped media
	}
	if strings.HasPrefix(trimmed, "<") || strings.Contains(ct, "text/html") {
		return fmt.Errorf("got HTML instead of media")
	}
	if strings.HasPrefix(ct, "image/") {
		return fmt.Errorf("got image stub (%s)", ct)
	}
	return nil
}

func stIsHLS(body string) bool {
	return strings.HasPrefix(strings.TrimLeft(body, " \t\r\n"), "#EXTM3U")
}

// stFirstURI returns the first URI line (non-comment) of a playlist body.
func stFirstURI(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

// stTagURI extracts URI="..." from the first tag line (e.g. "#EXT-X-KEY:").
// Reports ok=false when the tag is absent, has METHOD=NONE, or has no URI.
func stTagURI(body, tag string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, tag) {
			continue
		}
		if strings.Contains(line, "METHOD=NONE") {
			return "", false
		}
		const q = `URI="`
		i := strings.Index(line, q)
		if i < 0 {
			return "", false
		}
		rest := line[i+len(q):]
		j := strings.Index(rest, `"`)
		if j < 0 {
			return "", false
		}
		return rest[:j], true
	}
	return "", false
}

// stResolveRef resolves a playlist-relative reference against its base URL.
func stResolveRef(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

func stSetHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Origin", StreamoBaseURL)
	req.Header.Set("Referer", StreamoBaseURL+"/")
}
