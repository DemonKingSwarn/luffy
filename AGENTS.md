# AGENTS.md - LLM Agent Guidelines for Luffy

## Project Overview

**Luffy** is a CLI tool for streaming/downloading movies and TV shows from online providers. Written in Go 1.25.

**Key Features**: Search/stream from flixhq, sflix, braflix, movies4u, hdrezka, youtube. Interactive fzf selection. MPV/VLC/IINA support. yt-dlp downloads. Cross-platform (Linux, macOS, Windows, Android, FreeBSD).

## Build/Lint/Test Commands

```bash
# Build (development)
go build .
go install .

# Run with debug
go run . "movie title" --debug

# Run with specific provider
go run . "movie title" --provider sflix

# Auto-select best quality (skip fzf prompt)
go run . "movie title" --best

# Cross-platform builds (uses just)
just build                    # Build all platforms
just windows-amd64           # Specific platform
just mac-arm
just linux-amd64
just clean                   # Clean build directory

# Formatting
gofmt -w .                   # Format all Go files
go fmt ./...                 # Alternative format command

# Linting
go vet ./...                 # Go static analysis
golangci-lint run            # If golangci-lint is installed

# Testing
# Note: No test files currently exist in codebase
# When adding tests, use: go test ./...
# Run single test: go test -run TestFunctionName ./path/to/package
```

## Architecture

```
luffy/
├── cmd/root.go              # CLI entry point (cobra commands)
├── core/
│   ├── provider.go          # Provider interface
│   ├── types.go             # Core types (SearchResult, Season, Episode, Server)
│   ├── config.go            # Config management (YAML at ~/.config/luffy/config.yaml)
│   ├── decrypt.go           # M3U8 stream extraction (local decryption)
│   ├── m3u8.go              # Quality selection from master m3u8 playlists
│   ├── player.go            # Video player integration
│   ├── http.go              # HTTP client helpers
│   └── providers/           # Provider implementations
│       ├── flixhq.go        # Default provider
│       ├── sflix.go
│       ├── braflix.go
│       ├── movies4u.go      # Bollywood only
│       ├── hdrezka.go       # Experimental
│       └── youtube.go
└── main.go                  # Entry point
```

## Provider Interface

All providers implement `core.Provider`:
```go
type Provider interface {
    Search(query string) ([]SearchResult, error)
    GetMediaID(url string) (string, error)
    GetSeasons(mediaID string) ([]Season, error)
    GetEpisodes(id string, isSeason bool) ([]Episode, error)
    GetServers(episodeID string) ([]Server, error)
    GetLink(serverID string) (string, error)
}
```

## Quality Selection (core/m3u8.go)

After decryption, the returned stream URL is typically a **master m3u8 playlist** containing multiple quality variants. The quality selection system handles this transparently.

### Types

```go
type Quality struct {
    URL        string
    Resolution string  // e.g. "1920x1080"
    Bandwidth  int
    Height     int
    Label      string  // shown in fzf (resolution or bandwidth string)
}
```

### Functions

- `GetQualities(m3u8URL, client, referer string) ([]Quality, directURL string, error)`
  - Fetches the m3u8, passes `Referer` header (required by most CDNs)
  - If it's a master playlist (`#EXT-X-STREAM-INF`): returns all variant `Quality` structs
  - If it's a media playlist (no variants): returns `(nil, originalURL, nil)`
- `GetBestQuality(qualities []Quality) Quality` — returns highest resolution/bandwidth variant
- `SelectQuality(qualities []Quality, selectBest bool) (string, error)`
  - `selectBest=true`: auto-picks best (highest height, then bandwidth)
  - `selectBest=false`: launches fzf for user to pick; returns selected variant URL

### Quality Selection Flow in cmd/root.go

```go
if strings.Contains(streamURL, ".m3u8") {
    qualities, directURL, err := core.GetQualities(streamURL, ctx.Client, referer)
    if err == nil && len(qualities) > 0 {
        selectBest := bestFlag || strings.EqualFold(cfg.Quality, "best")
        streamURL, err = core.SelectQuality(qualities, selectBest)
    } else if directURL != "" {
        streamURL = directURL
    }
}
```

### Quality Selection Behavior

| Condition | Behavior |
|-----------|----------|
| `--best` flag | Auto-select highest quality, no fzf prompt |
| `quality: best` in config | Auto-select highest quality, no fzf prompt |
| Neither set (default) | fzf prompt shown, user picks quality |
| Single variant in playlist | Auto-selected, no fzf prompt |
| Not a master playlist | URL passed through as-is |

### Config Field

```yaml
# ~/.config/luffy/config.yaml
quality: best   # auto-select; omit or set to anything else to get fzf prompt
```

The default value for `Quality` in `core/config.go` is `""` (empty string), **not** `"best"`. This ensures fzf is shown by default. Only set `"best"` explicitly in the config or use `--best` to skip the prompt.

### Important: Referer Required for m3u8 Fetch

CDNs serving master playlists (e.g. from Megacloud/sflix/braflix) enforce `Referer` checks and return 403 without it. Always pass the `referer` computed in `processStream` (set after decryption) into `GetQualities`. Failing to do so causes `GetQualities` to return an error, qualities to be empty, and the master playlist URL to be passed directly to the player — bypassing quality selection silently.

## Stream Decryption (core/decrypt.go)

All stream decryption is done locally without external services. The `DecryptStream` function routes to specific decryptors based on URL patterns:

### Supported Embedders

| Embedder | Function | Notes |
|----------|----------|-------|
| `videostr.net` | `DecryptMegacloud` | Used by sflix, braflix |
| `streameeeeee.site` | `DecryptMegacloud` | Used by flixhq |
| `streamaaa.top` | `DecryptMegacloud` | Alternative |
| `megacloud.*` | `DecryptMegacloud` | Megacloud player |
| `embed.su` | `DecryptEmbedSu` | Embed.su player |
| `vidlink.pro` | `DecryptVidlink` | AES-256 encrypted API |
| `multiembed.mov` | `DecryptMultiembed` | Hunter obfuscation decoder |
| `vidsrc.*` | `DecryptVidsrc` | Cloudnestra-based |
| Other | `DecryptGeneric` | Regex-based fallback |

### Megacloud Decryption Flow

1. Fetch embed page HTML
2. Extract client key from HTML (multiple patterns):
   - `<meta name="_gg_fb" content="...">`
   - `window._xy_ws = "..."`
   - `window._lk_db = {x: "...", y: "...", z: "..."}`
   - `<div data-dpi="...">`
3. Fetch Megacloud key from public GitHub repo
4. Call API: `/embed-1/v3/e-1/getSources?id={videoID}&_k={clientKey}`
5. If encrypted, decrypt using 3-layer algorithm
6. Return m3u8 URL and subtitles

### Client Key Extraction Patterns

```go
patterns := []string{
    `<meta\s+name="_gg_fb"\s+content="([^"]+)"`,
    `window\._xy_ws\s*=\s*"([^"]+)"`,
    `window\._lk_db\s*=\s*\{[^}]*x:\s*"([^"]+)"[^}]*y:\s*"([^"]+)"[^}]*z:\s*"([^"]+)"`,
    `<div[^>]+data-dpi="([^"]+)"`,
}
```

### Adding New Embedder Support

1. Add URL pattern check in `DecryptStream()`
2. Implement decrypt function following existing patterns
3. Extract m3u8 URL from embed page or API response

## Code Conventions

### Imports

- Standard library first, then third-party, then internal
- Group imports logically
```go
import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"

    "github.com/PuerkitoBio/goquery"
    "github.com/spf13/cobra"

    "github.com/demonkingswarn/luffy/core"
)
```

### Formatting & Types

- Use `gofmt` for consistent formatting
- Use descriptive struct field names, JSON tags for API responses
```go
type SearchResult struct {
    Title  string
    URL    string
    Type   MediaType  // Movie or Series
    Poster string
    Year   string
}
```

### Naming Conventions

- **Constants**: UPPER_SNAKE_CASE (`FLIXHQ_BASE_URL`)
- **Public structs**: PascalCase (`FlixHQ`, `YouTube`)
- **Private structs**: PascalCase (`DecryptedSource`)
- **Receiver names**: Single lowercase letter matching type first letter (`f *FlixHQ`, `s *Sflix`)
- **Functions**: PascalCase for exported (`NewFlixHQ`), camelCase for private
- **Variables**: camelCase (`mediaType`, `searchURL`)

### Error Handling

- Always return errors from HTTP operations
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Return descriptive errors for empty results
```go
resp, err := p.Client.Do(req)
if err != nil {
    return nil, fmt.Errorf("failed to fetch data: %w", err)
}
defer resp.Body.Close()

if len(results) == 0 {
    return nil, errors.New("no results")
}
```

### Provider Implementation Pattern

```go
package providers

const (
    PROVIDER_BASE_URL = "https://example.com"
    PROVIDER_AJAX_URL = PROVIDER_BASE_URL + "/ajax"
)

type ProviderName struct {
    Client *http.Client
}

func NewProviderName(client *http.Client) *ProviderName {
    return &ProviderName{Client: client}
}

func (p *ProviderName) newRequest(method, url string) (*http.Request, error) {
    req, err := core.NewRequest(method, url)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Referer", PROVIDER_BASE_URL+"/")
    return req, nil
}
```

### HTTP Request Pattern

```go
func (p *Provider) Search(query string) ([]core.SearchResult, error) {
    search := strings.ReplaceAll(query, " ", "-")
    req, _ := p.newRequest("GET", PROVIDER_SEARCH_URL+"/"+search)
    resp, err := p.Client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    // Parse response...
}
```

### HTML Parsing with goquery

```go
doc, err := goquery.NewDocumentFromReader(resp.Body)
if err != nil {
    return nil, err
}

doc.Find("div.flw-item").Each(func(i int, sel *goquery.Selection) {
    title := sel.Find("h2.film-name a").AttrOr("title", "Unknown")
    href := sel.Find("div.film-poster a").AttrOr("href", "")
    // Process item...
})
```

### JSON Response Parsing

```go
var res struct {
    Type string `json:"type"`
    Link string `json:"link"`
}
if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
    return "", err
}
return res.Link, nil
```

### Server ID Stripping Pattern

For sflix and braflix, server IDs may include context suffixes (`|movie`, `|series`) that must be stripped before API calls:
```go
func (s *Sflix) GetLink(serverID string) (string, error) {
    id := serverID
    if idx := strings.Index(id, "|"); idx != -1 {
        id = id[:idx]
    }
    url := fmt.Sprintf("%s/episode/sources/%s", SFLIX_AJAX_URL, id)
    // ...
}
```

## Common Patterns

### Debug Output
```go
if ctx.Debug {
    fmt.Printf("Fetching URL: %s\n", url)
}
```

### HTTP Headers
- `User-Agent`: Set via `core.NewRequest()` or manually
- `Referer`: Set to provider base URL in `newRequest()`
- `X-Requested-With`: Set to "XMLHttpRequest" for AJAX calls

### Special Provider Handling

**sflix and braflix** need dynamic referrer in `cmd/root.go`:
```go
if strings.EqualFold(providerName, "sflix") || strings.EqualFold(providerName, "braflix") {
    if parsedURL, err := url.Parse(link); err == nil {
        referer = fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
    }
}
```

## Adding a New Provider

1. Create `core/providers/newprovider.go` implementing `Provider` interface
2. Add to switch statement in `cmd/root.go`:
```go
} else if strings.EqualFold(providerName, "newprovider") {
    provider = providers.NewNewProvider(client)
}
```
3. Update `README.md` with the new provider

## Important Notes

- **Never commit secrets** - No API keys in code
- **Respect rate limits** - Add delays if needed
- **Handle edge cases** - Empty results, network errors
- **Follow existing patterns** - Consistency across providers
- **Default provider** is flixhq
- **Config location**: `~/.config/luffy/config.yaml`
- **All decryption is local** - No external services used
- **Quality default is fzf prompt** - `cfg.Quality` defaults to `""`, not `"best"`. Only `--best` flag or explicit `quality: best` in config bypasses the prompt
- **Always pass Referer to GetQualities** - CDNs enforce Referer; missing it silently breaks quality selection

## Dependencies

- `github.com/PuerkitoBio/goquery` - HTML parsing
- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - Config parsing

## Resources

- GitHub: https://github.com/demonkingswarn/luffy
- Discord: https://discord.gg/JF85vTkDyC


## Architecture

```
luffy/
├── cmd/root.go              # CLI entry point (cobra commands)
├── core/
│   ├── provider.go          # Provider interface
│   ├── types.go             # Core types (SearchResult, Season, Episode, Server)
│   ├── config.go            # Config management (YAML at ~/.config/luffy/config.yaml)
│   ├── decrypt.go           # M3U8 stream extraction (local decryption)
│   ├── player.go            # Video player integration
│   ├── http.go              # HTTP client helpers
│   └── providers/           # Provider implementations
│       ├── flixhq.go        # Default provider
│       ├── sflix.go
│       ├── braflix.go
│       ├── movies4u.go      # Bollywood only
│       ├── hdrezka.go       # Experimental
│       └── youtube.go
└── main.go                  # Entry point
```

## Provider Interface

All providers implement `core.Provider`:
```go
type Provider interface {
    Search(query string) ([]SearchResult, error)
    GetMediaID(url string) (string, error)
    GetSeasons(mediaID string) ([]Season, error)
    GetEpisodes(id string, isSeason bool) ([]Episode, error)
    GetServers(episodeID string) ([]Server, error)
    GetLink(serverID string) (string, error)
}
```

## Stream Decryption (core/decrypt.go)

All stream decryption is done locally without external services. The `DecryptStream` function routes to specific decryptors based on URL patterns:

### Supported Embedders

| Embedder | Function | Notes |
|----------|----------|-------|
| `videostr.net` | `DecryptMegacloud` | Used by sflix, braflix |
| `streameeeeee.site` | `DecryptMegacloud` | Used by flixhq |
| `streamaaa.top` | `DecryptMegacloud` | Alternative |
| `megacloud.*` | `DecryptMegacloud` | Megacloud player |
| `embed.su` | `DecryptEmbedSu` | Embed.su player |
| `vidlink.pro` | `DecryptVidlink` | AES-256 encrypted API |
| `multiembed.mov` | `DecryptMultiembed` | Hunter obfuscation decoder |
| `vidsrc.*` | `DecryptVidsrc` | Cloudnestra-based |
| Other | `DecryptGeneric` | Regex-based fallback |

### Megacloud Decryption Flow

1. Fetch embed page HTML
2. Extract client key from HTML (multiple patterns):
   - `<meta name="_gg_fb" content="...">`
   - `window._xy_ws = "..."`
   - `window._lk_db = {x: "...", y: "...", z: "..."}`
   - `<div data-dpi="...">`
3. Fetch Megacloud key from public GitHub repo
4. Call API: `/embed-1/v3/e-1/getSources?id={videoID}&_k={clientKey}`
5. If encrypted, decrypt using 3-layer algorithm
6. Return m3u8 URL and subtitles

### Client Key Extraction Patterns

```go
patterns := []string{
    `<meta\s+name="_gg_fb"\s+content="([^"]+)"`,
    `window\._xy_ws\s*=\s*"([^"]+)"`,
    `window\._lk_db\s*=\s*\{[^}]*x:\s*"([^"]+)"[^}]*y:\s*"([^"]+)"[^}]*z:\s*"([^"]+)"`,
    `<div[^>]+data-dpi="([^"]+)"`,
}
```

### Adding New Embedder Support

1. Add URL pattern check in `DecryptStream()`
2. Implement decrypt function following existing patterns
3. Extract m3u8 URL from embed page or API response

## Code Conventions

### Imports

- Standard library first, then third-party, then internal
- Group imports logically
```go
import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"

    "github.com/PuerkitoBio/goquery"
    "github.com/spf13/cobra"

    "github.com/demonkingswarn/luffy/core"
)
```

### Formatting & Types

- Use `gofmt` for consistent formatting
- Use descriptive struct field names, JSON tags for API responses
```go
type SearchResult struct {
    Title  string
    URL    string
    Type   MediaType  // Movie or Series
    Poster string
    Year   string
}
```

### Naming Conventions

- **Constants**: UPPER_SNAKE_CASE (`FLIXHQ_BASE_URL`)
- **Public structs**: PascalCase (`FlixHQ`, `YouTube`)
- **Private structs**: PascalCase (`DecryptedSource`)
- **Receiver names**: Single lowercase letter matching type first letter (`f *FlixHQ`, `s *Sflix`)
- **Functions**: PascalCase for exported (`NewFlixHQ`), camelCase for private
- **Variables**: camelCase (`mediaType`, `searchURL`)

### Error Handling

- Always return errors from HTTP operations
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Return descriptive errors for empty results
```go
resp, err := p.Client.Do(req)
if err != nil {
    return nil, fmt.Errorf("failed to fetch data: %w", err)
}
defer resp.Body.Close()

if len(results) == 0 {
    return nil, errors.New("no results")
}
```

### Provider Implementation Pattern

```go
package providers

const (
    PROVIDER_BASE_URL = "https://example.com"
    PROVIDER_AJAX_URL = PROVIDER_BASE_URL + "/ajax"
)

type ProviderName struct {
    Client *http.Client
}

func NewProviderName(client *http.Client) *ProviderName {
    return &ProviderName{Client: client}
}

func (p *ProviderName) newRequest(method, url string) (*http.Request, error) {
    req, err := core.NewRequest(method, url)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Referer", PROVIDER_BASE_URL+"/")
    return req, nil
}
```

### HTTP Request Pattern

```go
func (p *Provider) Search(query string) ([]core.SearchResult, error) {
    search := strings.ReplaceAll(query, " ", "-")
    req, _ := p.newRequest("GET", PROVIDER_SEARCH_URL+"/"+search)
    resp, err := p.Client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    // Parse response...
}
```

### HTML Parsing with goquery

```go
doc, err := goquery.NewDocumentFromReader(resp.Body)
if err != nil {
    return nil, err
}

doc.Find("div.flw-item").Each(func(i int, sel *goquery.Selection) {
    title := sel.Find("h2.film-name a").AttrOr("title", "Unknown")
    href := sel.Find("div.film-poster a").AttrOr("href", "")
    // Process item...
})
```

### JSON Response Parsing

```go
var res struct {
    Type string `json:"type"`
    Link string `json:"link"`
}
if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
    return "", err
}
return res.Link, nil
```

### Server ID Stripping Pattern

For sflix and braflix, server IDs may include context suffixes (`|movie`, `|series`) that must be stripped before API calls:
```go
func (s *Sflix) GetLink(serverID string) (string, error) {
    id := serverID
    if idx := strings.Index(id, "|"); idx != -1 {
        id = id[:idx]
    }
    url := fmt.Sprintf("%s/episode/sources/%s", SFLIX_AJAX_URL, id)
    // ...
}
```

## Common Patterns

### Debug Output
```go
if ctx.Debug {
    fmt.Printf("Fetching URL: %s\n", url)
}
```

### HTTP Headers
- `User-Agent`: Set via `core.NewRequest()` or manually
- `Referer`: Set to provider base URL in `newRequest()`
- `X-Requested-With`: Set to "XMLHttpRequest" for AJAX calls

### Special Provider Handling

**sflix and braflix** need dynamic referrer in `cmd/root.go`:
```go
if strings.EqualFold(providerName, "sflix") || strings.EqualFold(providerName, "braflix") {
    if parsedURL, err := url.Parse(link); err == nil {
        referer = fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
    }
}
```

## Adding a New Provider

1. Create `core/providers/newprovider.go` implementing `Provider` interface
2. Add to switch statement in `cmd/root.go`:
```go
} else if strings.EqualFold(providerName, "newprovider") {
    provider = providers.NewNewProvider(client)
}
```
3. Update `README.md` with the new provider

## Important Notes

- **Never commit secrets** - No API keys in code
- **Respect rate limits** - Add delays if needed
- **Handle edge cases** - Empty results, network errors
- **Follow existing patterns** - Consistency across providers
- **Default provider** is flixhq
- **Config location**: `~/.config/luffy/config.yaml`
- **All decryption is local** - No external services used

## Dependencies

- `github.com/PuerkitoBio/goquery` - HTML parsing
- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - Config parsing

## Resources

- GitHub: https://github.com/demonkingswarn/luffy
- Discord: https://discord.gg/JF85vTkDyC
