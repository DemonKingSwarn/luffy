# Image / Poster Preview

Luffy can download and display movie or show posters in the terminal using [chafa](https://hpjansson.org/chafa/) before the fzf selection prompt. This is controlled by the `--show-image` flag and the `image_backend` config option.

## How It Works

### 1. Trigger

When `--show-image` is passed on the command line, poster preview is activated for two flows:

- **Normal search** — posters come from provider `SearchResult.Poster` fields (direct image URLs from the provider).
- **Recommendations** — posters come from TMDB via `TMDB_IMAGE_BASE_URL + Recommendation.PosterPath`.

### 2. Parallel Download

All posters are downloaded concurrently using a `sync.WaitGroup`. Each poster is fetched with `core.DownloadPoster`, which:

1. Resolves `~/.cache/luffy/` as the cache directory (creating it if absent).
2. Sanitizes the title to a safe filename by replacing any non-alphanumeric character with `_`, then appending `.jpg`.
3. Returns immediately if the file already exists (cache hit).
4. Otherwise fetches the URL with a standard `http.Client` (using `core.NewRequest` to set the `User-Agent`) and writes the body to disk.

```go
var wg sync.WaitGroup
for _, r := range results {
    wg.Add(1)
    go func(r core.SearchResult) {
        defer wg.Done()
        core.DownloadPoster(r.Poster, r.Title)
    }(r)
}
wg.Wait()
```

### 3. fzf Preview Command

After all posters are downloaded, `cmd/root.go` constructs a fzf preview command string and calls `core.SelectWithPreview`:

```go
previewCmd := fmt.Sprintf("%s preview --backend %s --cache %s {}", exeFwd, cfg.ImageBackend, cacheDirFwd)
idx = core.SelectWithPreview("Results:", titles, previewCmd)
```

The `{}` placeholder is fzf syntax — fzf substitutes the currently highlighted item's label before executing the command. The `preview` sub-command is a **hidden** cobra command (`previewCmd`) defined in `cmd/root.go`.

### 4. The `preview` Sub-command

When fzf invokes `luffy preview --backend <backend> --cache <dir> "<title>"`, the hidden `preview` command:

1. Strips the `[movie]` / `[series]` type prefix from the label using a regex (`^\[.*\] `).
2. Sanitizes the remaining title with the same `[^a-zA-Z0-9]+` → `_` rule used by `DownloadPoster`.
3. Constructs the full path: `<cache>/<safeTitle>.jpg`.
4. Calls `core.PreviewWithBackend(path, backend)`.

### 5. Rendering with chafa

`PreviewWithBackend` runs:

```
chafa -f <backend> <path>
```

Both `stdout` and `stderr` are connected to the terminal so chafa's output appears directly in the fzf preview pane. The `backend` string maps to a chafa output format:

| Backend value | Rendering method |
|---------------|-----------------|
| `sixel`       | Sixel graphics (default) |
| `kitty`       | Kitty terminal graphics protocol |
| `iterm`       | iTerm2 inline image protocol |
| `symbols`     | Unicode block/braille characters (no graphics protocol needed) |

The default backend is `sixel`. Override it in `~/.config/luffy/config.yaml`:

```yaml
image_backend: kitty
```

### 6. Cache Cleanup

After the fzf selection returns, `cmd/root.go` fires `core.CleanCache()` in a goroutine so it does not block the rest of the flow:

```go
go core.CleanCache()
```

`CleanCache` opens `~/.cache/luffy/`, reads all entries with `Readdirnames(-1)`, and calls `os.RemoveAll` on each one, leaving the directory itself intact.

## Key Types

This package does not define custom types. All functions operate on plain `string` paths and URLs.

## Public API

```go
// GetCacheDir returns ~/.cache/luffy/, creating it if it does not exist.
func GetCacheDir() (string, error)

// DownloadPoster fetches the image at url and saves it to the cache directory
// under a filename derived from title. Returns the full path to the saved file.
// Does nothing and returns the existing path if the file is already cached.
func DownloadPoster(url string, title string) (string, error)

// CleanCache removes all files from ~/.cache/luffy/.
func CleanCache() error

// PreviewPoster renders the image at path using the backend from the loaded
// config file. Calls PreviewWithBackend internally.
func PreviewPoster(path string) error

// PreviewWithBackend renders the image at path by running:
//   chafa -f <backend> <path>
// Output goes to stdout/stderr.
func PreviewWithBackend(path, backend string) error
```

## Internal Helpers

| Function | Purpose |
|----------|---------|
| `GetCacheDir` | Resolves and creates `~/.cache/luffy/` |

## End-to-End Flow Diagram

```
luffy "title" --show-image
        │
        ▼
provider.Search()  →  []SearchResult{Poster: "https://..."}
        │
        ▼  (parallel goroutines)
DownloadPoster(url, title)  →  ~/.cache/luffy/<safeTitle>.jpg
        │
        ▼
core.SelectWithPreview("Results:", labels, "luffy preview --backend sixel --cache ~/.cache/luffy {}")
        │                                          │
        │   fzf highlights an item                 │
        │ ─────────────────────────────────────►  │
        │                                          ▼
        │                              luffy preview --backend sixel
        │                                    --cache <dir> "[movie] Title"
        │                                          │
        │                                          ▼
        │                              PreviewWithBackend(path, "sixel")
        │                                          │
        │                                          ▼
        │                                   chafa -f sixel <path>
        │                                 (renders in fzf preview pane)
        │
        ▼  (user confirms selection)
go CleanCache()   ← runs in background
```
