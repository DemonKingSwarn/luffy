# Hooks & MpvArgs

Luffy supports user-defined shell commands that run at key playback lifecycle points, and extra mpv arguments that are passed on every invocation. Both are configured in `~/.config/luffy/config.yaml`.

## MpvArgs

`mpv_args` is a YAML list of strings appended verbatim to the mpv command line after all built-in flags. It lets you enable hardware decoding, set a default volume, pass mpv scripts, or any other mpv option — without recompiling.

```yaml
mpv_args:
  - "--hwdec=auto"
  - "--volume=80"
  - "--save-position-on-quit"
  - "--script=/home/user/.config/mpv/scripts/something.lua"
```

`mpv_args` is **ignored** on VLC, IINA, and Android. It only applies when mpv is the active player.

## Hooks

Hooks are shell command strings that run at three points in the playback lifecycle. Each hook is executed via `sh -c` on Unix or `cmd /c` on Windows, blocking until it completes. The current environment is inherited, with additional `LUFFY_*` variables set describing the current media.

Failures are printed to stderr but never abort playback or download — hooks are best-effort.

### Lifecycle Events

| Hook | Config key | When it fires |
|------|-----------|---------------|
| on_play | `hooks.on_play` | Just before the player process is launched |
| on_exit | `hooks.on_exit` | Immediately after the player process exits |
| on_download | `hooks.on_download` | Just before a download starts (yt-dlp or native) |

### Environment Variables

Every hook receives the following variables:

| Variable | Type | Description |
|----------|------|-------------|
| `LUFFY_TITLE` | string | Media title (movie or show name) |
| `LUFFY_URL` | string | Provider media URL (the page URL, not the stream) |
| `LUFFY_SEASON` | integer | Season number; `0` for movies |
| `LUFFY_EPISODE` | integer | Episode number within the season; `0` for movies |
| `LUFFY_EP_NAME` | string | Episode name; empty for movies |
| `LUFFY_PROVIDER` | string | Provider name (e.g. `flixhq`, `sflix`) |
| `LUFFY_ACTION` | string | `play` or `download` |
| `LUFFY_STREAM_URL` | string | Resolved stream URL (m3u8 or direct) |
| `LUFFY_POSITION` | float | Playback position in seconds at exit (`on_exit` only; `0.000` otherwise) |

### Config Schema

```yaml
hooks:
  on_play: '<shell command>'
  on_exit: '<shell command>'
  on_download: '<shell command>'
```

All three keys are optional. An empty or absent value disables that hook.

### Examples

**Desktop notification when playback starts:**
```yaml
hooks:
  on_play: 'notify-send "Now playing" "$LUFFY_TITLE"'
```

**Log every session to a file:**
```yaml
hooks:
  on_exit: |
    echo "$(date '+%F %T') | $LUFFY_TITLE S${LUFFY_SEASON}E${LUFFY_EPISODE} | stopped at ${LUFFY_POSITION}s" >> ~/luffy.log
```

**Update a now-playing status (e.g. Discord rich presence via a script):**
```yaml
hooks:
  on_play: '~/.local/bin/luffy-rich-presence set "$LUFFY_TITLE"'
  on_exit: '~/.local/bin/luffy-rich-presence clear'
```

**Notification before download:**
```yaml
hooks:
  on_download: 'notify-send "Downloading" "$LUFFY_TITLE — S${LUFFY_SEASON}E${LUFFY_EPISODE}"'
```

**All three combined:**
```yaml
mpv_args:
  - "--hwdec=auto"

hooks:
  on_play: 'notify-send "Now playing" "$LUFFY_TITLE"'
  on_exit: 'echo "$LUFFY_TITLE stopped at ${LUFFY_POSITION}s" >> ~/luffy.log'
  on_download: 'notify-send "Downloading" "$LUFFY_TITLE"'
```

## Implementation Details

### HookContext (core/hooks.go)

```go
type HookContext struct {
    Title     string
    URL       string  // provider media URL
    Season    int     // 0 for movies
    Episode   int     // 0 for movies
    EpName    string
    Provider  string
    Action    string  // "play" or "download"
    StreamURL string  // resolved stream URL
    Position  float64 // playback position in seconds (on_exit only)
}
```

### RunHook (core/hooks.go)

```go
// RunHook executes command in a shell with LUFFY_* env vars set.
// No-op if command is "". Errors are printed, never fatal.
func RunHook(command string, hctx HookContext, debug bool)
```

When `debug` is true, the command string is printed before execution.

### Call Sites

| Location | Hook fired | Notes |
|----------|-----------|-------|
| `core/player.go: Play` | `on_play`, `on_exit` | Fires before launch and after `cmd.Wait()` |
| `core/player.go: PlayWithControls` | `on_play`, `on_exit` | Fires each loop iteration (including Replay) |
| `cmd/root.go: buildProcessStream` | `on_download` | Fires before `Download` / `DownloadYTDLP` |

`HookContext` must be populated at the call site in `cmd/root.go`. The player functions fill in `Action` and `StreamURL` themselves; the caller provides `Title`, `URL`, `Provider`, `Season`, `Episode`, and `EpName`.
