# AGENTS.md - LLM Agent Guidelines for Luffy

## Project Overview

**Luffy** is a CLI tool for streaming/downloading movies and TV shows from online providers. Written in Go 1.25.

**Key Features**: Search/stream from flixhq, sflix, braflix, brocoflix, xprime, movies4u, hdrezka, youtube. Interactive fzf selection. MPV/VLC/IINA support. yt-dlp downloads. Cross-platform (Linux, macOS, Windows, Android, FreeBSD).

## Build/Lint/Test Commands

```bash
# Build (development)
go build .
go install .

# Run with debug
go run . "movie title" --debug

# Run with specific provider
go run . "movie title" --provider sflix

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
│   ├── decrypt.go           # M3U8 stream extraction
│   ├── player.go            # Video player integration
│   ├── http.go              # HTTP client helpers
│   └── providers/           # Provider implementations
│       ├── flixhq.go        # Default provider
│       ├── sflix.go
│       ├── braflix.go
│       ├── brocoflix.go
│       ├── xprime.go
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

### MediaID Context Pattern (sflix/braflix)

Format: `"id|mediaID"` to pass media type context:
```go
func (p *Provider) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
    parts := strings.Split(id, "|")
    actualID := parts[0]
    mediaID := ""
    if len(parts) == 2 {
        mediaID = parts[1]
    }
    // Use actualID for API calls, append mediaID to returned episode IDs
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

## Dependencies

- `github.com/PuerkitoBio/goquery` - HTML parsing
- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - Config parsing

## Resources

- GitHub: https://github.com/demonkingswarn/luffy
- Discord: https://discord.gg/JF85vTkDyC
