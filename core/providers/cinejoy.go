package providers

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/demonkingswarn/luffy/core"
	"github.com/tetratelabs/wazero"
)

const (
	// CinejoyBaseURL is the site origin used for Search result URLs, metadata,
	// and the Referer/Origin values required by Cinejoy CDNs during playback.
	CinejoyBaseURL = "https://cinejoy.to"
	// CinejoyAPI is the resolver endpoint family used for sealed requests.
	CinejoyAPI = "https://api.shegu.st"

	// cjWASMSHA256 pins the upstream import-free encoder. A changed encoder
	// fails closed and requires inspection before the pin is updated.
	cjWASMSHA256 = "b706f1f2d21ef20d49758672dbdf3559b0d0916bb19850b0ef04991918eea73c"

	cjMaxConcurrency   = 3
	cjHTTPTimeout      = 15 * time.Second
	cjSealTimeout      = 5 * time.Second
	cjRaceTimeout      = 60 * time.Second
	cjPerServerTimeout = 25 * time.Second
	cjAuthBodyLimit    = 4 << 20
	cjPlaylistLimit    = 4 << 20
	cjSegmentLimit     = 16 << 20
	cjCatalogTTL       = 5 * time.Minute
)

// cjVerifiedServers are the catalog servers proven by MAD-881 to resolve and
// decode test media. Other advertised servers stay disabled until verified.
var cjVerifiedServers = []string{"Lisbon", "Nebula", "Solara"}

// Cinejoy is a browser-free Cinejoy provider: metadata comes from TMDB, stream
// resolution uses the pinned upstream crush.wasm encoder through wazero.
type Cinejoy struct {
	Client *http.Client

	mu        sync.Mutex
	wasm      []byte
	catalog   []cjServer
	catalogAt time.Time
}

type cjServer struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type cjEnvelope struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
}

type cjSealed struct {
	responseKey     []byte
	keyID           byte
	ephemeralPublic []byte
	body            []byte
}

type cjStream struct {
	Playlist  string               `json:"playlist"`
	Type      string               `json:"type"`
	Qualities map[string]cjQuality `json:"qualities"`
	Captions  []json.RawMessage    `json:"captions"`
	ID        json.RawMessage      `json:"id"`
}

type cjQuality struct {
	URL string `json:"url"`
}

type cjCandidate struct {
	url      string
	captions int
}

type cjRequest struct {
	kind    string // movie | series
	tmdb    string
	season  string
	episode string
	title   string
	year    string
}

func NewCinejoy(client *http.Client) *Cinejoy {
	return &Cinejoy{Client: client}
}

// --- ID encoding -----------------------------------------------------------

// cinejoyID builds the composite ID carried through the Provider interface:
//
//	movie:  movie|<tmdb>|<title>|<year>
//	series: series|<tmdb>|<season>|<episode>|<title>|<year>
//
// Pipe characters are removed from free-text parts so the encoding stays
// unambiguous.
func cinejoyID(cj cjRequest) string {
	sanitize := func(s string) string {
		return strings.ReplaceAll(s, "|", "-")
	}
	if cj.kind == "series" {
		return strings.Join([]string{"series", cj.tmdb, cj.season, cj.episode, sanitize(cj.title), cj.year}, "|")
	}
	return strings.Join([]string{"movie", cj.tmdb, sanitize(cj.title), cj.year}, "|")
}

func parseCinejoyID(id string) (cj cjRequest, err error) {
	parts := strings.Split(id, "|")
	if len(parts) < 2 {
		return cj, fmt.Errorf("invalid cinejoy ID")
	}
	cj.kind = parts[0]
	cj.tmdb = parts[1]
	if cj.tmdb == "" || !(cj.kind == "movie" || cj.kind == "series") {
		return cj, fmt.Errorf("invalid cinejoy ID")
	}
	switch cj.kind {
	case "movie":
		if len(parts) > 2 {
			cj.title = parts[2]
		}
		if len(parts) > 3 {
			cj.year = parts[3]
		}
	case "series":
		// Series IDs come in two shapes:
		//   show:  series|<tmdb>|<title>|<year>            (GetSeasons input)
		//   ep:    series|<tmdb>|<season>|<episode>|<title>|<year>
		// Season/episode presence is enforced by resolve/GetEpisodes.
		if len(parts) > 2 {
			cj.season = parts[2]
		}
		if len(parts) > 3 {
			cj.episode = parts[3]
		}
		if len(parts) > 4 {
			cj.title = parts[4]
		}
		if len(parts) > 5 {
			cj.year = parts[5]
		}
	}
	return cj, nil
}

// --- Provider interface -----------------------------------------------------

func (c *Cinejoy) Search(query string) ([]core.SearchResult, error) {
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
	resp, err := c.Client.Do(req)
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
			URL:    fmt.Sprintf("%s/%s/%d?title=%s&year=%s", CinejoyBaseURL, r.MediaType, r.ID, url.QueryEscape(title), year),
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

func (c *Cinejoy) GetMediaID(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid cinejoy URL")
	}
	kind := parts[0]
	if kind == "tv" {
		kind = "series"
	} else if kind != "movie" {
		return "", fmt.Errorf("invalid cinejoy URL")
	}
	return cinejoyID(cjRequest{
		kind:  kind,
		tmdb:  parts[1],
		title: u.Query().Get("title"),
		year:  u.Query().Get("year"),
	}), nil
}

func (c *Cinejoy) GetSeasons(mediaID string) ([]core.Season, error) {
	cj, err := parseCinejoyID(mediaID)
	if err != nil {
		return nil, err
	}
	if cj.kind != "series" {
		return nil, nil
	}

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s?api_key=%s", core.TMDB_BASE_URL, cj.tmdb, core.TMDB_API_KEY))
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data core.TmdbShowDetails
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var seasons []core.Season
	for _, s := range data.Seasons {
		if s.SeasonNumber == 0 {
			continue
		}
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("Season %d", s.SeasonNumber)
		}
		seasons = append(seasons, core.Season{
			ID:   cinejoyID(cjRequest{kind: "series", tmdb: cj.tmdb, season: fmt.Sprintf("%d", s.SeasonNumber), title: cj.title, year: cj.year}),
			Name: name,
		})
	}
	return seasons, nil
}

func (c *Cinejoy) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	if !isSeason {
		// Movie path (recommendation flow): the episode list IS the server list.
		return []core.Episode{{ID: id, Name: "Cinejoy"}}, nil
	}

	cj, err := parseCinejoyID(id)
	if err != nil {
		return nil, err
	}
	if cj.kind != "series" || cj.season == "" {
		return nil, fmt.Errorf("invalid cinejoy season ID")
	}

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s/season/%s?api_key=%s", core.TMDB_BASE_URL, cj.tmdb, cj.season, core.TMDB_API_KEY))
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
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
			ID:   cinejoyID(cjRequest{kind: "series", tmdb: cj.tmdb, season: cj.season, episode: fmt.Sprintf("%d", e.EpisodeNumber), title: cj.title, year: cj.year}),
			Name: fmt.Sprintf("E%02d - %s", e.EpisodeNumber, e.Name),
		})
	}
	return episodes, nil
}

func (c *Cinejoy) GetServers(id string) ([]core.Server, error) {
	// Resolution automatically races the verified catalog servers and returns
	// the first validated stream, so a single auto server entry is exposed.
	// The caller-supplied ID carries the full request identity (movie tmdb id,
	// or series tmdb+season+episode) and is passed through to GetLink.
	return []core.Server{{ID: id, Name: "Cinejoy (Auto)"}}, nil
}

func (c *Cinejoy) GetLink(serverID string) (string, error) {
	cj, err := parseCinejoyID(serverID)
	if err != nil {
		return "", err
	}
	if cj.kind == "series" && (cj.season == "" || cj.episode == "") {
		return "", fmt.Errorf("cinejoy: series resolution requires season and episode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cjRaceTimeout)
	defer cancel()

	wasm, err := c.fetchWASM(ctx)
	if err != nil {
		return "", err
	}
	servers, err := c.fetchCatalog(ctx)
	if err != nil {
		return "", err
	}
	if len(servers) == 0 {
		return "", fmt.Errorf("cinejoy: no verified servers available")
	}

	params := map[string]any{"tmdb": cj.tmdb}
	if cj.kind == "series" {
		params["season"] = cj.season
		params["episode"] = cj.episode
	}
	if cj.title != "" {
		params["title"] = cj.title
	}
	if cj.year != "" {
		params["year"] = cj.year
	}

	winner := make(chan cjCandidate, 1)
	var errMu sync.Mutex
	var failures []string

	next := 0
	workers := cjMaxConcurrency
	if workers > len(servers) {
		workers = len(servers)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var server cjServer
				c.mu.Lock()
				if next < len(servers) {
					server = servers[next]
					next++
				}
				c.mu.Unlock()
				if server.Name == "" {
					return
				}
				cand, serr := c.tryServer(ctx, wasm, server.Name, cj.kind, params)
				if serr != nil {
					errMu.Lock()
					failures = append(failures, fmt.Sprintf("%s: %v", server.Name, serr))
					errMu.Unlock()
					continue
				}
				select {
				case winner <- cand:
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
	case cand := <-winner:
		return cand.url, nil
	default:
		errMu.Lock()
		msg := strings.Join(failures, "; ")
		errMu.Unlock()
		if msg == "" {
			msg = "all attempts cancelled"
		}
		return "", fmt.Errorf("cinejoy: no playable stream: %s", msg)
	}
}

// --- Cached sources ---------------------------------------------------------

func (c *Cinejoy) fetchWASM(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	if c.wasm != nil {
		c.mu.Unlock()
		return c.wasm, nil
	}
	c.mu.Unlock()

	body, err := cjFetch(ctx, c.Client, http.MethodGet, CinejoyAPI+"/crush.wasm", nil, 4<<20)
	if err != nil {
		return nil, fmt.Errorf("cinejoy: fetch encoder: %w", err)
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != cjWASMSHA256 {
		return nil, fmt.Errorf("cinejoy: encoder hash mismatch; upstream changed, refuse to run")
	}

	c.mu.Lock()
	if c.wasm == nil {
		c.wasm = body
	}
	res := c.wasm
	c.mu.Unlock()
	return res, nil
}

func (c *Cinejoy) fetchCatalog(ctx context.Context) ([]cjServer, error) {
	c.mu.Lock()
	if c.catalog != nil && time.Since(c.catalogAt) < cjCatalogTTL {
		c.mu.Unlock()
		return c.catalog, nil
	}
	c.mu.Unlock()

	body, err := cjFetch(ctx, c.Client, http.MethodGet, CinejoyAPI+"/servers", nil, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("cinejoy: fetch catalog: %w", err)
	}
	var data struct {
		Servers []cjServer `json:"servers"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("cinejoy: invalid catalog: %w", err)
	}

	verified := make(map[string]bool, len(cjVerifiedServers))
	for _, name := range cjVerifiedServers {
		verified[name] = true
	}
	var ready []cjServer
	for _, s := range data.Servers {
		if strings.EqualFold(s.Status, "ok") && verified[s.Name] {
			ready = append(ready, s)
		}
	}

	c.mu.Lock()
	c.catalog = ready
	c.catalogAt = time.Now()
	c.mu.Unlock()
	return ready, nil
}

// --- Per-server resolution --------------------------------------------------

func (c *Cinejoy) tryServer(parent context.Context, wasm []byte, name, kind string, params map[string]any) (cjCandidate, error) {
	ctx, cancel := context.WithTimeout(parent, cjPerServerTimeout)
	defer cancel()

	sealed, err := cjSealRequest(ctx, wasm, "/"+name+"/"+kind, params)
	if err != nil {
		return cjCandidate{}, err
	}

	envelope, err := cjResolve(ctx, c.Client, sealed)
	if err != nil {
		return cjCandidate{}, err
	}
	if envelope.Status < 200 || envelope.Status >= 300 {
		return cjCandidate{}, fmt.Errorf("resolver status %d", envelope.Status)
	}

	candidates := cjExtractCandidates(envelope.Data)
	if len(candidates) == 0 {
		// Nested embed resolution, limited to the first two stream entries.
		for _, s := range cjStreams(envelope.Data) {
			if s.Playlist != "" && len(s.ID) > 0 && !bytes.Equal(s.ID, []byte("null")) {
				nested, nerr := c.cjResolveNested(ctx, c.Client, wasm, name, kind, copyParams(params, s.ID))
				if nerr != nil {
					continue
				}
				candidates = append(candidates, nested...)
				if len(candidates) >= 2 {
					break
				}
			}
		}
	}
	if len(candidates) == 0 {
		return cjCandidate{}, fmt.Errorf("no stream candidates")
	}

	if err := cjValidateStream(ctx, c.Client, candidates[0].url); err != nil {
		return cjCandidate{}, err
	}
	return candidates[0], nil
}

func (c *Cinejoy) cjResolveNested(ctx context.Context, client *http.Client, wasm []byte, name, kind string, params map[string]any) ([]cjCandidate, error) {
	sealed, err := cjSealRequest(ctx, wasm, "/"+name+"/"+kind, params)
	if err != nil {
		return nil, err
	}
	envelope, err := cjResolve(ctx, client, sealed)
	if err != nil {
		return nil, err
	}
	if envelope.Status < 200 || envelope.Status >= 300 {
		return nil, fmt.Errorf("resolver status %d", envelope.Status)
	}
	return cjExtractCandidates(envelope.Data), nil
}

// --- Protocol helpers -------------------------------------------------------

func cjStreams(data json.RawMessage) []cjStream {
	var parsed struct {
		Stream []cjStream `json:"stream"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	if parsed.Stream == nil {
		// Ensure "[]" (no streams) and absent field behave the same.
		return nil
	}
	return parsed.Stream
}

func cjExtractCandidates(data json.RawMessage) []cjCandidate {
	streams := cjStreams(data)
	var out []cjCandidate
	for _, s := range streams {
		if strings.HasPrefix(s.Playlist, "https://") {
			out = append(out, cjCandidate{url: s.Playlist, captions: len(s.Captions)})
			continue
		}
		if s.Type == "file" && s.Qualities != nil {
			type q struct {
				key string
				url string
			}
			var entries []q
			for k, v := range s.Qualities {
				if v.URL != "" {
					entries = append(entries, q{key: k, url: v.URL})
				}
			}
			sort.Slice(entries, func(i, j int) bool {
				ai, aerr := cjParseInt(entries[i].key)
				bi, berr := cjParseInt(entries[j].key)
				if aerr != nil {
					return false
				}
				if berr != nil {
					return true
				}
				return ai > bi
			})
			for _, e := range entries {
				out = append(out, cjCandidate{url: e.url, captions: len(s.Captions)})
			}
		}
	}
	return out
}

// cjParseInt parses digits-only ints used as quality keys.
func cjParseInt(s string) (int, error) {
	var n int
	if s == "" {
		return 0, fmt.Errorf("empty integer")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not an integer")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func copyParams(base map[string]any, embed json.RawMessage) map[string]any {
	params := make(map[string]any, len(base)+1)
	for k, v := range base {
		params[k] = v
	}
	// embed must be a plain string or number for the sealed payload.
	if len(embed) > 0 {
		var p any
		if json.Unmarshal(embed, &p) == nil {
			switch p.(type) {
			case string, float64:
				params["embed"] = p
			}
		}
	}
	return params
}

func cjSealRequest(ctx context.Context, wasm []byte, path string, payload map[string]any) (*cjSealed, error) {
	body, err := json.Marshal(map[string]any{"path": path, "payload": payload})
	if err != nil {
		return nil, err
	}

	sealCtx, sealCancel := context.WithTimeout(ctx, cjSealTimeout)
	defer sealCancel()

	rt := wazero.NewRuntime(sealCtx)
	defer rt.Close(sealCtx)

	compiled, err := rt.CompileModule(sealCtx, wasm)
	if err != nil {
		return nil, fmt.Errorf("encoder compile: %w", err)
	}
	if len(compiled.ImportedFunctions()) > 0 || len(compiled.ImportedMemories()) > 0 {
		return nil, fmt.Errorf("encoder has host imports; refuse to run")
	}
	if len(compiled.ExportedMemories()) == 0 {
		return nil, fmt.Errorf("encoder has no exported memory")
	}

	mod, err := rt.InstantiateModule(sealCtx, compiled, wazero.NewModuleConfig().WithStartFunctions())
	if err != nil {
		return nil, fmt.Errorf("encoder instantiate: %w", err)
	}

	alloc := mod.ExportedFunction("alloc")
	if alloc == nil {
		return nil, fmt.Errorf("encoder missing alloc export")
	}
	seal := mod.ExportedFunction("seal_request")
	if seal == nil {
		return nil, fmt.Errorf("encoder missing seal_request export")
	}

	seed := make([]byte, 44)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("encoder seed: %w", err)
	}
	capLen := uint64(len(body) + 512)

	p, err := alloc.Call(sealCtx, uint64(len(body)))
	if err != nil || len(p) == 0 || p[0] == 0 {
		return nil, fmt.Errorf("encoder alloc failed")
	}
	s, err := alloc.Call(sealCtx, uint64(len(seed)))
	if err != nil || len(s) == 0 || s[0] == 0 {
		return nil, fmt.Errorf("encoder seed alloc failed")
	}
	o, err := alloc.Call(sealCtx, capLen)
	if err != nil || len(o) == 0 || o[0] == 0 {
		return nil, fmt.Errorf("encoder output alloc failed")
	}

	mem := mod.Memory()
	if !mem.Write(uint32(p[0]), body) || !mem.Write(uint32(s[0]), seed) {
		return nil, fmt.Errorf("encoder memory write failed")
	}

	n, err := seal.Call(sealCtx, p[0], uint64(len(body)), s[0], uint64(len(seed)), o[0], capLen)
	if err != nil {
		return nil, fmt.Errorf("encoder seal failed")
	}
	length := n[0]
	if length <= 98 || length > capLen {
		return nil, fmt.Errorf("encoder seal returned invalid length")
	}
	out, ok := mem.Read(uint32(o[0]), uint32(length))
	if !ok {
		return nil, fmt.Errorf("encoder output read failed")
	}
	return splitCinejoySealed(out), nil
}

func splitCinejoySealed(bytes []byte) *cjSealed {
	if len(bytes) <= 98 {
		panic("malformed sealed request") // guarded by caller length check
	}
	return &cjSealed{
		responseKey:     bytes[:32],
		keyID:           bytes[32],
		ephemeralPublic: bytes[33:98],
		body:            bytes[98:],
	}
}

func cinejoyAAD(s *cjSealed) []byte {
	aad := make([]byte, 0, 14+len(s.ephemeralPublic))
	aad = append(aad, "lumen-gate-v2"...)
	aad = append(aad, 0, 2, s.keyID)
	aad = append(aad, s.ephemeralPublic...)
	return aad
}

func cjResolve(ctx context.Context, client *http.Client, sealed *cjSealed) (*cjEnvelope, error) {
	body, err := cjFetch(ctx, client, http.MethodPost, CinejoyAPI+"/g", sealed.body, cjAuthBodyLimit, "text/plain;charset=UTF-8")
	if err != nil {
		return nil, err
	}
	envelope, err := cjDecrypt(sealed, body)
	if err != nil {
		return nil, err
	}
	return &envelope, nil
}

func cjDecrypt(sealed *cjSealed, wire []byte) (cjEnvelope, error) {
	var envelope cjEnvelope
	if len(wire) < 28 {
		return envelope, fmt.Errorf("malformed encrypted response")
	}
	block, err := aes.NewCipher(sealed.responseKey)
	if err != nil {
		return envelope, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return envelope, err
	}
	plain, err := gcm.Open(nil, wire[:12], wire[12:], cinejoyAAD(sealed))
	if err != nil {
		return envelope, fmt.Errorf("response authentication failed")
	}
	if err := json.Unmarshal(plain, &envelope); err != nil {
		return envelope, fmt.Errorf("invalid response envelope")
	}
	return envelope, nil
}

// cjValidateStream proves a candidate is playable: HLS playlists are followed
// to a media playlist and a bounded first segment; direct files are rejected
// (not supported by the probe scope either).
func cjValidateStream(ctx context.Context, client *http.Client, rawURL string) error {
	current := rawURL
	for depth := 0; depth < 3; depth++ {
		text, err := cjFetchText(ctx, client, current, cjPlaylistLimit)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(text, "#EXTM3U") {
			return fmt.Errorf("not an HLS playlist")
		}
		var media string
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				media = line
				break
			}
		}
		if media == "" {
			return fmt.Errorf("empty playlist")
		}
		if strings.Contains(text, "#EXT-X-STREAM-INF:") {
			next, err := url.Parse(current)
			if err != nil {
				return err
			}
			ref, err := url.Parse(media)
			if err != nil {
				return err
			}
			current = next.ResolveReference(ref).String()
			continue
		}
		playlistURL, err := url.Parse(current)
		if err != nil {
			return err
		}
		ref, err := url.Parse(media)
		if err != nil {
			return err
		}
		segment := playlistURL.ResolveReference(ref).String()
		bytes, err := cjFetch(ctx, client, http.MethodGet, segment, nil, cjSegmentLimit)
		if err != nil {
			return err
		}
		if len(bytes) < 188 {
			return fmt.Errorf("invalid media segment")
		}
		for _, prefix := range []string{"<!doctype", "<html", "{"} {
			if strings.HasPrefix(strings.ToLower(string(bytes[:100])), prefix) {
				return fmt.Errorf("invalid media segment")
			}
		}
		return nil
	}
	return fmt.Errorf("playlist nesting exceeds limit")
}

func cjFetchText(ctx context.Context, client *http.Client, rawURL string, limit int64) (string, error) {
	body, err := cjFetch(ctx, client, http.MethodGet, rawURL, nil, limit)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// cjFetch is the hardened HTTP helper: HTTPS only, no redirects, no local
// hosts, bounded bodies, and Cinejoy request headers on every call.
func cjFetch(ctx context.Context, client *http.Client, method, rawURL string, body []byte, limit int64, contentType ...string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "https" || u.User != nil {
		return nil, fmt.Errorf("unsupported URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || !strings.Contains(host, ".") || isNumericHost(host) || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || host == "localhost" {
		return nil, fmt.Errorf("unsupported URL")
	}

	reqCtx, cancel := context.WithTimeout(ctx, cjHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Origin", CinejoyBaseURL)
	req.Header.Set("Referer", CinejoyBaseURL+"/")
	if len(contentType) > 0 && contentType[0] != "" {
		req.Header.Set("Content-Type", contentType[0])
	}

	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := clientCopy.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// isNumericHost rejects raw-IP targets (probe parity).
func isNumericHost(host string) bool {
	return net.ParseIP(host) != nil
}
