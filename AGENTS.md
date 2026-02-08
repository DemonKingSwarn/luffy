# AGENTS.md - LLM Agent Guidelines for Luffy

## Project Overview

**Luffy** is a CLI tool for streaming and downloading movies and TV shows. It's written in Go and serves as a spiritual successor to flix-cli and mov-cli.

### Key Features
- Search and stream movies/TV shows from various online sources
- Support for multiple providers (flixhq, sflix, braflix, brocoflix, xprime, movies4u, hdrezka, youtube)
- Interactive selection using fzf
- MPV/VLC/IINA player support
- Download support via yt-dlp
- Poster preview support (with chafa/libsixel)
- Cross-platform (Linux, macOS, Windows, Android, FreeBSD)

## Architecture

```
luffy/
├── cmd/
│   └── root.go          # CLI commands and main application logic
├── core/
│   ├── provider.go      # Provider interface definition
│   ├── types.go         # Core type definitions
│   ├── config.go        # Configuration management
│   ├── decrypt.go       # Stream decryption logic
│   ├── player.go        # Video player integration
│   ├── downloader.go    # yt-dlp download integration
│   ├── http.go          # HTTP client utilities
│   ├── input.go         # User input/prompts
│   ├── episodes.go      # Episode range parsing
│   ├── m3u8.go          # M3U8 stream handling
│   ├── image.go         # Poster image handling
│   ├── context.go       # Application context
│   └── providers/       # Provider implementations
│       ├── flixhq.go    # Default provider
│       ├── sflix.go     
│       ├── braflix.go   
│       ├── brocoflix.go
│       ├── xprime.go
│       ├── movies4u.go  # Bollywood only
│       ├── hdrezka.go   # Experimental
│       └── youtube.go
├── main.go              # Entry point
└── README.md
```

## Provider Interface

All providers must implement the `Provider` interface defined in `core/provider.go`:

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

### Core Types

```go
// SearchResult represents a search result
type SearchResult struct {
    Title  string
    URL    string
    Type   MediaType  // Movie or Series
    Poster string
    Year   string
}

// Season represents a TV show season
type Season struct {
    ID   string
    Name string
}

// Episode represents a TV episode or movie server
type Episode struct {
    ID   string
    Name string
}

// Server represents a streaming server
type Server struct {
    ID   string
    Name string
}
```

## Code Conventions

### 1. Provider Implementation Pattern

Each provider follows this structure:

```go
package providers

import (
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "strings"
    
    "github.com/PuerkitoBio/goquery"
    "github.com/demonkingswarn/luffy/core"
)

const (
    PROVIDER_BASE_URL   = "https://example.com"
    PROVIDER_SEARCH_URL = PROVIDER_BASE_URL + "/search"
    PROVIDER_AJAX_URL   = PROVIDER_BASE_URL + "/ajax"
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

### 2. HTTP Request Pattern

Always use the `newRequest` helper method to ensure consistent headers:

```go
func (p *Provider) Search(query string) ([]core.SearchResult, error) {
    search := strings.ReplaceAll(query, " ", "-")
    req, _ := p.newRequest("GET", PROVIDER_SEARCH_URL+"/"+search)
    resp, err := p.Client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    // ... parse response
}
```

### 3. HTML Parsing with goquery

Use goquery for HTML parsing:

```go
doc, err := goquery.NewDocumentFromReader(resp.Body)
if err != nil {
    return nil, err
}

var results []core.SearchResult
doc.Find("div.flw-item").Each(func(i int, sel *goquery.Selection) {
    title := sel.Find("h2.film-name a").AttrOr("title", "Unknown")
    href := sel.Find("div.film-poster a").AttrOr("href", "")
    // ...
})
```

### 4. MediaID Context Pattern (for sflix/braflix)

For providers that need media type context, append mediaID to IDs:

```go
// Format: "id|mediaID"
func (p *Provider) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
    var actualID, mediaID string
    parts := strings.Split(id, "|")
    if len(parts) == 2 {
        actualID = parts[0]
        mediaID = parts[1]
    } else {
        actualID = id
    }
    // ... use actualID for API calls
    // ... append mediaID to returned episode IDs
}
```

### 5. JSON Response Parsing

Use structured types for JSON responses:

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

## Provider-Specific Guidelines

### Default Provider (flixhq)
- Located at `core/providers/flixhq.go`
- Base URL: `https://flixhq.to`
- Uses standard ajax endpoints

### Experimental Providers
- **hdrezka**: `core/providers/hdrezka.go` - Russian provider

### Special Handling

**sflix and braflix** have special referrer handling in `cmd/root.go`:
```go
// Use main URL of embed link as referrer
if strings.EqualFold(providerName, "sflix") || strings.EqualFold(providerName, "braflix") {
    if parsedURL, err := url.Parse(link); err == nil {
        referer = fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
    }
}
```

## Stream Decryption

The `core/decrypt.go` file handles extracting m3u8 URLs from embed links:

```go
func DecryptStream(embedLink string, client *http.Client) (string, []string, string, error)
```

Returns:
- m3u8 URL
- Subtitle URLs
- Referer
- Error

Providers should call this after `GetLink()` to get the actual stream URL.

## Adding a New Provider

1. Create a new file in `core/providers/newprovider.go`
2. Implement the `Provider` interface
3. Add the provider to the switch statement in `cmd/root.go`:

```go
} else if strings.EqualFold(providerName, "newprovider") {
    provider = providers.NewNewProvider(client)
}
```

4. Update `README.md` with the new provider

## Testing Guidelines

1. Always test search functionality first
2. Test both movies and TV shows
3. Test season/episode selection for TV shows
4. Test server selection
5. Verify stream URLs are valid
6. Check subtitle extraction if applicable

## Build and Run

### Using Just (Recommended for Releases)

The project uses [just](https://github.com/casey/just) as its build runner. The `justfile` contains recipes for building cross-platform binaries:

```bash
# Build all platforms
just build

# Build specific platform
just windows-amd64
just linux-amd64
just mac-arm
just mac-intel

# Clean build directory
just clean
```

**Available build targets:**
- `windows-amd64`, `windows-386`, `windows-arm`
- `linux-amd64`, `linux-386`, `linux-arm`, `linux-risc`
- `mac-arm`, `mac-intel`
- `freebsd-amd64`, `freebsd-386`

Built binaries are placed in the `builds/` directory with UPX compression applied where available.

### Using Go Directly (Development)

```bash
# Build
go build .

# Install
go install .

# Run with debug
go run . "movie title" --debug

# Run with specific provider
go run . "movie title" --provider sflix
```

## Dependencies

External dependencies used:
- `github.com/PuerkitoBio/goquery` - HTML parsing
- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - Config file parsing

External tools required:
- `mpv`/`vlc`/`iina` - Video playback
- `yt-dlp` - Downloads
- `fzf` - Interactive selection
- `chafa`/`libsixel` - Image display (optional)

## Common Patterns

### Error Handling
Always return descriptive errors:
```go
if err != nil {
    return nil, fmt.Errorf("failed to fetch data: %w", err)
}
```

### Debug Output
Use debug flag for verbose output:
```go
if ctx.Debug {
    fmt.Printf("Fetching URL: %s\n", url)
}
```

### HTTP Headers
Set appropriate headers:
```go
req.Header.Set("User-Agent", "Mozilla/5.0 ...")
req.Header.Set("Referer", baseURL)
req.Header.Set("X-Requested-With", "XMLHttpRequest") // For AJAX calls
```

## Important Notes

1. **Never commit secrets** - No API keys in code
2. **Respect rate limits** - Add delays if needed
3. **Handle edge cases** - Empty results, network errors
4. **Test on all platforms** - Linux, macOS, Windows, Android
5. **Follow existing patterns** - Consistency across providers
6. **Update documentation** - README.md and this file

## Resources

- GitHub: https://github.com/demonkingswarn/luffy
- Discord: https://discord.gg/JF85vTkDyC
- Issues: Create GitHub issues for bugs/features
