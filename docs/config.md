# Configuration

`core/config.go` loads user preferences from a YAML file and exposes them as a `Config` struct. All other packages call `LoadConfig()` at the point they need a setting; there is no global singleton or initialisation step.

## How It Works

### 1. Config File Location

The config file is read from:

```
~/.config/luffy/config.yaml
```

If the file does not exist, cannot be read, or contains invalid YAML, `LoadConfig` silently returns a struct populated with default values. No error is surfaced to the user.

### 2. Defaults

| Field | Default | Notes |
|-------|---------|-------|
| `fzf_path` | `"fzf"` | PATH lookup; set to an absolute path if fzf is not on PATH |
| `player` | `"mpv"` | `"vlc"` selects VLC; `"mpv"` selects mpv; IINA is used automatically on macOS |
| `image_backend` | `"sixel"` | chafa rendering backend for poster previews |
| `provider` | `"flixhq"` | Default search provider when `--provider` flag is not given |
| `dl_path` | `""` | Download destination; empty means the user's home directory |
| `quality` | `""` | Empty means show fzf prompt; set to `"best"` to auto-select highest quality |
| `mpv_args` | `[]` | Extra CLI arguments appended to every mpv invocation |
| `hooks.on_play` | `""` | Shell command run before the player launches |
| `hooks.on_exit` | `""` | Shell command run after the player exits |
| `hooks.on_download` | `""` | Shell command run before a download starts |

### 3. YAML Schema

A fully-specified config file:

```yaml
fzf_path: /usr/local/bin/fzf
player: mpv
image_backend: kitty
provider: sflix
dl_path: /home/user/Movies
quality: best

mpv_args:
  - "--hwdec=auto"
  - "--volume=80"

hooks:
  on_play: 'notify-send "Now playing" "$LUFFY_TITLE"'
  on_exit: 'echo "$LUFFY_TITLE stopped at ${LUFFY_POSITION}s" >> ~/luffy.log'
  on_download: 'notify-send "Downloading" "$LUFFY_TITLE"'
```

All fields are optional. Any field not present in the file retains its default.

### 4. Quality Field Semantics

The `quality` field has special semantics:

- `""` (default, field absent from config): fzf quality selection prompt is shown every time a stream is played.
- `"best"`: highest-resolution variant is auto-selected; no fzf prompt.
- Any other value: treated as `""` (fzf prompt shown).

The `--best` CLI flag overrides this field at runtime.

### 5. MpvArgs Field

`mpv_args` is a list of strings appended verbatim to the mpv command line after all built-in flags (referrer, user-agent, subtitle files, IPC socket, start position). This lets users enable hardware decoding, set a default volume, pass mpv scripts, etc. without modifying the binary.

This field is **only applied to mpv**. It is silently ignored on VLC, IINA, and Android.

```yaml
mpv_args:
  - "--hwdec=auto"
  - "--volume=80"
  - "--script=/home/user/.config/mpv/scripts/something.lua"
```

### 6. Hooks Fields

See `docs/hooks.md` for the full hooks reference.

`hooks.on_play`, `hooks.on_exit`, and `hooks.on_download` each accept a single shell command string. An empty string (the default) disables the hook. Hooks run via `sh -c` on Unix or `cmd /c` on Windows, inherit the current environment, and have access to `LUFFY_*` environment variables describing the current media.

### 7. Image Backend Field

Used by `core/image.go` when calling `chafa` to render poster images in the terminal. Common values:

| Value | Description |
|-------|-------------|
| `sixel` | Sixel graphics (default; works in xterm, mlterm, WezTerm) |
| `kitty` | Kitty graphics protocol |
| `iterm` | iTerm2 inline images |
| `symbols` | Unicode block characters (no graphics protocol needed) |

## Key Types

```go
// HooksConfig holds shell commands to run at specific playback lifecycle points.
type HooksConfig struct {
    OnPlay     string `yaml:"on_play"`
    OnExit     string `yaml:"on_exit"`
    OnDownload string `yaml:"on_download"`
}

// Config holds all user-configurable preferences.
type Config struct {
    FzfPath      string      `yaml:"fzf_path"`      // path to fzf binary
    Player       string      `yaml:"player"`         // "mpv" or "vlc"
    ImageBackend string      `yaml:"image_backend"`  // chafa backend for poster display
    Provider     string      `yaml:"provider"`       // default search provider
    DlPath       string      `yaml:"dl_path"`        // download destination directory
    Quality      string      `yaml:"quality"`        // "" or "best"
    MpvArgs      []string    `yaml:"mpv_args"`       // extra args appended to mpv
    Hooks        HooksConfig `yaml:"hooks"`          // lifecycle hook commands
}
```

## Public API

```go
// LoadConfig reads ~/.config/luffy/config.yaml and returns a *Config.
// Returns a defaults-only Config if the file is missing or unreadable.
// Never returns nil.
func LoadConfig() *Config
```
