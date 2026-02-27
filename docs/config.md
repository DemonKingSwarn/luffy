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
| `player` | `"mpv"` | `"vlc"` selects VLC; `"mpv"` selects mpv; `"iina"` is used automatically on macOS |
| `image_backend` | `"sixel"` | chafa rendering backend for poster previews |
| `provider` | `"flixhq"` | Default search provider when `--provider` flag is not given |
| `dl_path` | `""` | Download destination; empty means the user's home directory |
| `quality` | `""` | Empty means show fzf prompt; set to `"best"` to auto-select highest quality |

### 3. YAML Schema

A fully-specified config file:

```yaml
fzf_path: /usr/local/bin/fzf
player: vlc
image_backend: kitty
provider: sflix
dl_path: /home/user/Movies
quality: best
```

All fields are optional. Any field not present in the file retains its default.

### 4. Quality Field Semantics

The `quality` field has special semantics:

- `""` (default, field absent from config): fzf quality selection prompt is shown every time a stream is played.
- `"best"`: highest-resolution variant is auto-selected; no fzf prompt.
- Any other value: treated as `""` (fzf prompt shown).

The `--best` CLI flag overrides this field at runtime.

### 5. Image Backend Field

Used by `core/image.go` when calling `chafa` to render poster images in the terminal. Common values:

| Value | Description |
|-------|-------------|
| `sixel` | Sixel graphics (default; works in xterm, mlterm, WezTerm) |
| `kitty` | Kitty graphics protocol |
| `iterm` | iTerm2 inline images |
| `symbols` | Unicode block characters (no graphics protocol needed) |

## Key Types

```go
// Config holds all user-configurable preferences.
type Config struct {
    FzfPath      string `yaml:"fzf_path"`      // path to fzf binary
    Player       string `yaml:"player"`         // "mpv" or "vlc"
    ImageBackend string `yaml:"image_backend"`  // chafa backend for poster display
    Provider     string `yaml:"provider"`       // default search provider
    DlPath       string `yaml:"dl_path"`        // download destination directory
    Quality      string `yaml:"quality"`        // "" or "best"
}
```

## Public API

```go
// LoadConfig reads ~/.config/luffy/config.yaml and returns a *Config.
// Returns a defaults-only Config if the file is missing or unreadable.
// Never returns nil.
func LoadConfig() *Config
```
