# CLI Orchestration

`cmd/root.go` is the entry point for all user-facing logic. It wires together providers, history, recommendations, decryption, quality selection, and playback into a single cobra command.

## Command Structure

```
luffy [query]            # main command — search, stream, or download
luffy preview [title]    # hidden sub-command used by fzf preview integration
```

## Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--season` | `-s` | int | 0 | Pre-select a season by number |
| `--episodes` | `-e` | string | `""` | Single episode or range (`1`, `1-5`) |
| `--action` | `-a` | string | `""` | Skip action prompt (`play` or `download`) |
| `--show-image` | | bool | false | Display poster thumbnails in fzf via chafa |
| `--provider` | `-p` | string | `""` | Override config provider |
| `--debug` | `-d` | bool | false | Print verbose debug output |
| `--best` | `-b` | bool | false | Auto-select highest quality, skip fzf prompt |
| `--history` | `-H` | bool | false | Resume from watch history |
| `--recommend` | `-r` | bool | false | Show TMDB recommendations from history |

## How It Works

The `RunE` function follows three mutually exclusive top-level branches, then converges on a shared stream resolution and playback pipeline.

### Branch 1: `--history`

1. Open the history DB; call `ListShows()` to get one row per unique title, newest first.
2. Present the list in fzf via `core.Select`.
3. Reconstruct the correct provider from `ShowSummary.Provider` (falls back to current config if blank).
4. Call `GetMediaID` on the stored URL.
5. Probe `GetSeasons` — if it returns seasons, treat as Series; otherwise treat as Movie.
6. For Series: auto-select the last-watched season if `ShowSummary.Season > 0`, then fzf-select an episode.
7. For Movie: enumerate servers via `GetEpisodes(mediaID, false)`.
8. Fall into the shared action/playback pipeline (see below).

### Branch 2: `--recommend`

1. Call `core.GetRecommendations(client)` to build a scored list from history (see `docs/recommendation.md`).
2. If `--show-image`: download all TMDB posters concurrently, then call `core.SelectWithPreview`; otherwise `core.Select`.
3. Auto-match the chosen title against provider search results (exact title + media type preferred).
4. Fall into the shared season/episode/action/playback pipeline (see Normal Search Flow below).

### Branch 3: Normal Search Flow

1. If no positional args: prompt with `core.Prompt("Search")`; otherwise join args with a space.
2. Call `provider.Search(query)`; display results as `[movie] Title` / `[series] Title` labels.
3. If `--show-image`: download provider posters concurrently, then `core.SelectWithPreview`; otherwise `core.Select`.
4. Resolve `GetMediaID(selected.URL)`.
5. For sflix: append `|movie` or `|series` to the media ID.

#### Season / Episode Selection

| Condition | Behaviour |
|-----------|-----------|
| `-s N` provided | Skip season fzf prompt; select season N directly |
| `-e N` or `-e N-M` provided | Skip episode fzf prompt; build episode list from `ParseEpisodeRange` |
| Neither | fzf prompt for season, then fzf prompt for episode |

Episode numbers are tracked via the `episodeWithNum` struct (1-based within the season). The index into `allEpisodesInSeason` is stored as `selectedEpisodeIdx` to seed the playback controls loop.

#### Action Selection

If `--action` is not set, fzf prompts `["Play", "Download"]`. The chosen value is lowercased to `"play"` or `"download"`.

### Shared Playback Pipeline

#### Movie path

1. Prefer a server whose name contains `"vidcloud"` (case-insensitive); fall back to the first server. For HDRezka take the first server unconditionally.
2. Call `provider.GetLink(serverID)` to get the raw embed/stream URL.
3. Pass the raw link to `resolveStreamURL` (see below).
4. **Play**: call `core.PlayWithControls`; save history on `PlaybackQuit`.
5. **Download**: call `buildProcessStream` which routes to `core.Download` or `core.DownloadYTDLP`.

#### Series path (play)

Call `playSeriesWithControls`, which runs an infinite loop:

```
for {
    getLinkForEpisode → resolveStreamURL → PlayWithControls
    saveHistory
    switch action {
        Quit     → return
        Next     → idx++
        Previous → idx--
        Replay   → stay (PlayWithControls handles internally)
    }
}
```

Navigation clamps at 0 and `len(allEpisodes)-1`. On a link/stream error the episode is skipped and the loop advances.

#### Series path (download)

Iterate `episodesToProcess`; for each episode call `GetServers → GetLink → buildProcessStream`. Errors are logged and skipped.

## Key Internal Functions

### `resolveStreamURL`

```go
func resolveStreamURL(
    link string,
    ctx *core.Context,
    cfg *core.Config,
    providerName string,
    name string,
    debugMode bool,
    best bool,
) (streamURL, referer string, subtitles []string, err error)
```

Converts a raw provider link into a final playable URL:

1. **HDRezka**: parses the comma-separated `[quality]url` format; picks the highest numeric quality.
2. **movies4u / youtube**: passes the link through unchanged.
3. **All others**: calls `core.DecryptStream` (see `docs/decrypt.md`). For sflix/braflix the referer is derived from the embed link's host.
4. If the resulting URL contains `.m3u8`: calls `core.GetQualities` with the correct `Referer` header, then `core.SelectQuality` (auto or fzf depending on `--best` / `quality: best`).

### `buildProcessStream`

Returns a closure `func(link, name string, season, episode int, epName string) error` that:

1. Calls `resolveStreamURL`.
2. Dispatches to `core.Play` (play) or `core.Download` / `core.DownloadYTDLP` (download).
3. Calls `saveHistory` on success.

Used for single-episode downloads and as the download loop body for series.

### `getLinkForEpisode`

```go
func getLinkForEpisode(ewn episodeWithNum, prov core.Provider, providerName string) (string, error)
```

Fetches servers for an episode ID, prefers `"vidcloud"` (except HDRezka), and returns `GetLink(serverID)`.

### `saveHistory`

```go
func saveHistory(histDB *core.DB, ctx *core.Context, providerName string, season, episode int, epName string, debugMode bool)
```

Writes one `core.HistoryEntry` to the SQLite DB. No-ops silently if `histDB` is nil.

### `playSeriesWithControls`

```go
func playSeriesWithControls(
    allEpisodes []episodeWithNum,
    startIdx int,
    seasonNum int,
    prov core.Provider,
    ctx *core.Context,
    cfg *core.Config,
    providerName string,
    histDB *core.DB,
    debugMode bool,
    best bool,
) error
```

The series playback loop. Calls `getLinkForEpisode → resolveStreamURL → core.PlayWithControls` for each episode and handles `PlaybackNext`, `PlaybackPrevious`, `PlaybackReplay`, and `PlaybackQuit` actions.

## The `preview` Sub-command

```
luffy preview [title] --backend <fmt> --cache <dir>
```

Hidden from help output. Invoked exclusively by the fzf preview command string built in `cmd/root.go`. Strips the `[movie]`/`[series]` prefix, sanitizes the title, constructs the cache path, and calls `core.PreviewWithBackend`. See `docs/image.md` for full details.

## Provider Resolution

Provider is selected in this priority order:

1. `--provider` flag
2. `provider` field in `~/.config/luffy/config.yaml`
3. Default: FlixHQ

```go
if providerFlag != "" {
    providerName = providerFlag
} else {
    providerName = cfg.Provider
}
```

The switch that constructs the `core.Provider` value is a simple `strings.EqualFold` chain; unrecognized names fall through to FlixHQ.

## `episodeWithNum`

```go
type episodeWithNum struct {
    num int        // 1-based episode number within the season
    ep  core.Episode
}
```

`core.Episode` has no `Number` field. The correct 1-based episode number is attached at the point of selection and carried through to history persistence. **Never** use a loop index from `episodesToProcess` as the episode number; use `episodeWithNum.num`.

## Internal Helpers

| Function | Purpose |
|----------|---------|
| `resolveStreamURL` | Converts a raw link → final playable URL + referer + subtitles |
| `buildProcessStream` | Returns a closure for decrypt → play/download → save history |
| `getLinkForEpisode` | Fetches servers for an episode and returns the preferred link |
| `saveHistory` | Writes a `HistoryEntry` to SQLite; no-ops if DB is nil |
| `playSeriesWithControls` | Series playback loop with Next/Previous/Replay/Quit navigation |
