package cinebyproviders

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "embed"

	"github.com/demonkingswarn/luffy/core"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed module1.wasm
var wasmModule []byte

const (
	VIDEASY_API_URL  = "https://api.videasy.net"
	VIDEASY_DB_URL   = "https://db.videasy.net/3"
	VIDEASY_REFERER  = "https://www.vidking.net/"
)

type VideasySource struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
}

type VideasySubtitle struct {
	URL      string `json:"url"`
	Language string `json:"language"`
	Lang     string `json:"lang"`
}

type VideasyResponse struct {
	Sources    []VideasySource    `json:"sources"`
	Subtitles []VideasySubtitle  `json:"subtitles"`
}

type VideasyProvider struct {
	Client *http.Client
}

func NewVideasy(client *http.Client) *VideasyProvider {
	return &VideasyProvider{Client: client}
}

func (v *VideasyProvider) Search(query string) ([]core.SearchResult, error) {
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
	req.Header.Set("Referer", "https://www.vidking.net/")

	resp, err := v.Client.Do(req)
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
			URL:    fmt.Sprintf("/%s/%d?title=%s&year=%s", r.MediaType, r.ID, url.QueryEscape(title), year),
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

func (v *VideasyProvider) GetMediaID(mediaURL string) (string, error) {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid videasy URL")
	}
	mediaType := parts[0]
	if mediaType == "tv" {
		mediaType = "series"
	}
	return fmt.Sprintf("%s|%s|%s|%s", parts[1], mediaType, u.Query().Get("title"), u.Query().Get("year")), nil
}

func (v *VideasyProvider) GetSeasons(mediaID string) ([]core.Season, error) {
	parts := strings.Split(mediaID, "|")
	if len(parts) < 2 || parts[1] != "series" {
		return nil, nil
	}

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s?language=en-US", VIDEASY_DB_URL, parts[0]))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", VIDEASY_REFERER)

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Seasons []struct {
			SeasonNumber int    `json:"season_number"`
			Name         string `json:"name"`
		} `json:"seasons"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	var seasons []core.Season
	for _, s := range data.Seasons {
		if s.SeasonNumber == 0 {
			continue
		}
		seasons = append(seasons, core.Season{
			ID:   fmt.Sprintf("%s|%s|%d|%s|%s", parts[0], parts[1], s.SeasonNumber, safePart(parts, 2), safePart(parts, 3)),
			Name: s.Name,
		})
	}
	return seasons, nil
}

func (v *VideasyProvider) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	parts := strings.Split(id, "|")
	if !isSeason {
		return []core.Episode{{ID: fmt.Sprintf("%s|%s|0|0|%s|%s", parts[0], safePart(parts, 1), safePart(parts, 2), safePart(parts, 3)), Name: "Movie"}}, nil
	}
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid videasy season ID")
	}

	req, err := core.NewRequest("GET", fmt.Sprintf("%s/tv/%s/season/%s?language=en-US", VIDEASY_DB_URL, parts[0], parts[2]))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", VIDEASY_REFERER)

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Episodes []struct {
			EpisodeNumber int    `json:"episode_number"`
			Name          string `json:"name"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	var episodes []core.Episode
	for _, e := range data.Episodes {
		episodes = append(episodes, core.Episode{
			ID:   fmt.Sprintf("%s|%s|%s|%d|%s|%s", parts[0], parts[1], parts[2], e.EpisodeNumber, safePart(parts, 3), safePart(parts, 4)),
			Name: fmt.Sprintf("E%02d - %s", e.EpisodeNumber, e.Name),
		})
	}
	return episodes, nil
}

func (v *VideasyProvider) GetServers(episodeID string) ([]core.Server, error) {
	return []core.Server{{
		ID:   episodeID,
		Name: "Videasy",
	}}, nil
}

func (v *VideasyProvider) GetLink(serverID string) (string, error) {
	parts := strings.Split(serverID, "|")
	if len(parts) < 4 {
		return "", fmt.Errorf("invalid videasy server ID")
	}

	tmdbID := parts[0]
	mediaType := parts[1]
	if mediaType == "series" {
		mediaType = "tv"
	}
	title := safePart(parts, len(parts)-2)
	year := safePart(parts, len(parts)-1)

	// Extract season/episode from ID if present (6-part episode ID format)
	seasonID := ""
	episodeID := ""
	if len(parts) >= 6 {
		if parts[2] != "0" || parts[3] != "0" {
			seasonID = parts[2]
			episodeID = parts[3]
		}
	}

	// Use first available source provider
	endpoints := []string{
		"mb-flix/sources-with-title",
		"cdn/sources-with-title",
		"downloader2/sources-with-title",
		"1movies/sources-with-title",
	}

	var lastErr error
	for _, endpoint := range endpoints {
		apiURL := fmt.Sprintf("%s/%s?title=%s&mediaType=%s&tmdbId=%s&year=%s&_t=%d",
			VIDEASY_API_URL, endpoint, url.QueryEscape(title), mediaType, tmdbID, url.QueryEscape(year), time.Now().UnixMilli())
		if seasonID != "" {
			apiURL += fmt.Sprintf("&seasonId=%s&episodeId=%s", seasonID, episodeID)
		}

		req, err := core.NewRequest("GET", apiURL)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Referer", VIDEASY_REFERER)

		resp, err := v.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		encryptedHex := strings.TrimSpace(string(body))
		if encryptedHex == "" || len(encryptedHex) < 100 {
			lastErr = fmt.Errorf("empty or short response")
			continue
		}

		tmdbIDInt, _ := parseInt(tmdbID)
		decrypted, err := DecryptVideasy(encryptedHex, tmdbIDInt)
		if err != nil {
			lastErr = err
			continue
		}

		var data VideasyResponse
		if err := json.Unmarshal([]byte(decrypted), &data); err != nil {
			lastErr = err
			continue
		}

		// Return the best quality source with subtitles
		var bestURL string
		for _, src := range data.Sources {
			if strings.Contains(src.URL, ".m3u8") {
				bestURL = src.URL
				if src.Quality == "1080p" {
					break
				}
			}
		}
		if bestURL == "" && len(data.Sources) > 0 {
			bestURL = data.Sources[0].URL
		}
		if bestURL == "" {
			lastErr = fmt.Errorf("no m3u8 source found")
			continue
		}

		if len(data.Subtitles) > 0 {
			var subURLs []string
			for _, sub := range data.Subtitles {
				subURLs = append(subURLs, sub.URL)
			}
			return bestURL + "|subs=" + url.QueryEscape(strings.Join(subURLs, "\n")), nil
		}
		return bestURL, nil
	}

	return "", fmt.Errorf("all source providers failed: %w", lastErr)
}

func DecryptVideasy(encryptedHex string, tmdbId int) (string, error) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	envBuilder := r.NewHostModuleBuilder("env")
	envBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module) float64 { return 12345.6789 }).
		Export("seed")
	envBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, msg, file, line, col uint32) {}).
		Export("abort")
	if _, err := envBuilder.Instantiate(ctx); err != nil {
		return "", fmt.Errorf("env instantiate: %w", err)
	}

	mod, err := r.InstantiateWithConfig(ctx, wasmModule, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return "", fmt.Errorf("wasm instantiate: %w", err)
	}
	defer mod.Close(ctx)

	if _, err := mod.ExportedFunction("serve").Call(ctx); err != nil {
		return "", fmt.Errorf("serve: %w", err)
	}

	verifyFn := mod.ExportedFunction("verify")
	emptyPtr := writeWasmString(ctx, mod, "")
	if _, err := verifyFn.Call(ctx, uint64(emptyPtr)); err != nil {
		return "", fmt.Errorf("verify: %w", err)
	}

	encPtr := writeWasmString(ctx, mod, encryptedHex)
	decryptFn := mod.ExportedFunction("decrypt")
	floatBits := math.Float64bits(float64(tmdbId))
	dr, err := decryptFn.Call(ctx, uint64(encPtr), floatBits)
	if err != nil {
		return "", fmt.Errorf("wasm decrypt: %w", err)
	}
	if len(dr) == 0 {
		return "", fmt.Errorf("wasm decrypt returned empty")
	}

	firstLayer := readWasmString(mod, uint32(dr[0]))
	if firstLayer == "" {
		return "", fmt.Errorf("first layer decryption failed")
	}

	return openSSLAESDecrypt(firstLayer)
}

func openSSLAESDecrypt(base64Data string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Data))
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	if len(ciphertext) < 16 || string(ciphertext[:8]) != "Salted__" {
		return "", fmt.Errorf("not OpenSSL salted format")
	}

	salt := ciphertext[8:16]
	encrypted := ciphertext[16:]

	key, iv := evpBytesToKey([]byte{}, salt, 32, 16)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes new cipher: %w", err)
	}

	if len(encrypted)%aes.BlockSize != 0 {
		return "", fmt.Errorf("ciphertext not multiple of block size: %d", len(encrypted))
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(encrypted))
	mode.CryptBlocks(plaintext, encrypted)

	padding := int(plaintext[len(plaintext)-1])
	if padding > 0 && padding <= len(plaintext) {
		plaintext = plaintext[:len(plaintext)-padding]
	}

	return string(plaintext), nil
}

func evpBytesToKey(password, salt []byte, keyLen, ivLen int) ([]byte, []byte) {
	var derived []byte
	var hashInput []byte

	for len(derived) < keyLen+ivLen {
		hasher := md5.New()
		if len(hashInput) > 0 {
			hasher.Write(hashInput)
		}
		hasher.Write(password)
		hasher.Write(salt)
		hash := hasher.Sum(nil)
		derived = append(derived, hash...)
		hashInput = hash
	}

	return derived[:keyLen], derived[keyLen : keyLen+ivLen]
}

func readWasmString(mod api.Module, ptr uint32) string {
	mem := mod.Memory()
	if mem == nil || ptr == 0 {
		return ""
	}
	lenBytes, ok := mem.Read(ptr-4, 4)
	if !ok {
		return ""
	}
	byteLen := binary.LittleEndian.Uint32(lenBytes)
	if byteLen == 0 {
		return ""
	}
	charLen := byteLen >> 1
	data, ok := mem.Read(ptr, byteLen)
	if !ok {
		return ""
	}
	runes := make([]rune, charLen)
	for i := uint32(0); i < charLen; i++ {
		runes[i] = rune(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return string(runes)
}

func writeWasmString(ctx context.Context, mod api.Module, s string) uint32 {
	if s == "" {
		return 0
	}
	newFn := mod.ExportedFunction("__new")
	byteLen := len(s) * 2
	results, err := newFn.Call(ctx, uint64(byteLen), 2)
	if err != nil || len(results) == 0 {
		return 0
	}
	ptr := uint32(results[0])
	mem := mod.Memory()
	lenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBytes, uint32(byteLen))
	mem.Write(ptr-4, lenBytes)
	data := make([]byte, byteLen)
	for i, r := range s {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(r))
	}
	mem.Write(ptr, data)
	return ptr
}

func safePart(parts []string, idx int) string {
	if idx >= len(parts) {
		return ""
	}
	return parts[idx]
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
