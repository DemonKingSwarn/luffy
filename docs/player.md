# Player

`core/player.go` abstracts video player invocation across all supported platforms. It offers three modes: a simple blocking call, a background launch with suppressed output, and a full playback control loop with an fzf menu. Lifecycle hooks (`on_play`, `on_exit`) are fired automatically by `Play` and `PlayWithControls`.

## How It Works

### 1. Player Command Construction

`buildPlayerCmd` assembles the OS-level command but does not start the process. Platform detection runs in this order:

1. **Android** — detected by running `uname -o` and checking for the string `Android`. Launches VLC via the Android Activity Manager (`am start`) with an `android.intent.action.VIEW` intent.
2. **macOS** — uses `iina` with `--mpv-*` prefix flags.
3. **Linux / Windows / FreeBSD** — checks `cfg.Player`; uses VLC if set to `"vlc"`, otherwise defaults to mpv.

Player executables are `mpv` / `vlc` on non-Windows systems and `mpv.exe` / `vlc.exe` on Windows.

Common arguments passed to all desktop players:

| Argument | Purpose |
|----------|---------|
| `--referrer` / `--http-referrer` / `--mpv-referrer` | CDN Referer header for the stream |
| `--user-agent` / `--http-user-agent` / `--mpv-user-agent` | HTTP User-Agent for the stream |
| `--force-media-title` / `--meta-title` / `--mpv-force-media-title` | Title shown in the player window |
| `--sub-file` / `--input-slave` / `--mpv-sub-files` | Subtitle file URLs |

`cfg.MpvArgs` is appended verbatim after all built-in mpv flags. It is ignored on VLC, IINA, and Android.

### 2. Lifecycle Hooks

`Play` and `PlayWithControls` fire lifecycle hooks at two points:

- **`on_play`** — called with `Action = "play"` and `StreamURL` set, immediately before the player process is launched.
- **`on_exit`** — called after the player process exits, with `Position` set to the final playback position in seconds (0 if IPC tracking was unavailable).

Both functions accept a `HookContext` from the caller. The caller must populate `Title`, `URL`, `Provider`, `Season`, `Episode`, and `EpName` — these cannot be inferred inside `player.go`. `Action` and `StreamURL` are filled in automatically.

The `on_download` hook is not fired here; it is fired in `cmd/root.go`'s `buildProcessStream` before `Download` / `DownloadYTDLP`.

### 3. Blocking Play (`Play`)

`Play` loads config, fires `on_play`, connects the player's stdout and stderr to the terminal, and calls `cmd.Wait()`, blocking until the player exits. When IPC tracking is active, the final position is captured with one last IPC read after the process exits. `on_exit` is called with the final position before returning.

### 4. Background Launch (`StartPlayer`)

`StartPlayer` calls `cmd.Start()` instead of `cmd.Run()` and sets `cmd.Stdout = nil` and `cmd.Stderr = nil` so the player's output is discarded. This keeps the terminal free for the fzf control menu. Returns the running `*exec.Cmd` so the caller can wait on it or kill it.

### 5. Playback Control Loop (`PlayWithControls`)

`PlayWithControls` is the main entrypoint for series playback. It loads config once, then runs a loop:

1. Fire `on_play` hook.
2. Call `StartPlayer` to launch the player in the background.
3. Spawn a goroutine that waits for the player process to exit naturally and closes a `done` channel.
4. Call `SelectActionCtx("Playback:", ...)` to show an fzf menu with four options:
   - **Next** — move to the next episode.
   - **Previous** — move to the previous episode.
   - **Replay** — restart the same episode (loop back).
   - **Quit** — exit.
5. When the user picks an action (or the player exits naturally), kill the player if still running.
6. Capture final IPC position, fire `on_exit` hook with `Position` set.
7. If `Replay` is chosen, restart the loop from step 1 (hooks fire again).

The fzf menu appears immediately after the player launches, so the user can navigate episodes while content plays — or after it finishes naturally.

### 6. IPC Position Tracking

When mpv is used on a supported platform, `buildPlayerCmd` adds `--input-ipc-server=<socketPath>`. A background goroutine polls `time-pos` via gopv every second, updating `lastPos`. On player exit, one final IPC read is attempted to capture the last position. The socket file is cleaned up with `os.Remove` in all paths.

## Key Types

```go
// PlaybackAction is the value returned by PlayWithControls to tell
// the caller what to do next.
type PlaybackAction string

const (
    PlaybackReplay   PlaybackAction = "Replay"
    PlaybackNext     PlaybackAction = "Next"
    PlaybackPrevious PlaybackAction = "Previous"
    PlaybackQuit     PlaybackAction = "Quit"
)

// PlayResult bundles the chosen action and the final playback position.
type PlayResult struct {
    Action       PlaybackAction
    PositionSecs float64 // seconds; 0 if not tracked
}
```

## Public API

```go
// Play launches the player and blocks until it exits.
// Fires on_play before launch and on_exit after exit.
// hctx must have Title, URL, Provider, Season, Episode, EpName populated.
// Returns final playback position in seconds (0 if IPC unavailable).
func Play(url, title, referer, userAgent string, subtitles []string, debug bool, startSecs float64, hctx HookContext) (float64, error)

// StartPlayer launches the player in the background with output suppressed.
// cfg is used for MpvArgs; pass nil to call LoadConfig() internally.
// Returns the running *exec.Cmd. The caller is responsible for cmd.Wait().
func StartPlayer(url, title, referer, userAgent string, subtitles []string, debug bool, socketPath string, startSecs float64, cfg *Config) (*exec.Cmd, error)

// PlayWithControls starts the player in the background and shows an fzf
// menu (Next / Previous / Replay / Quit). Fires on_play and on_exit each
// iteration. hctx must have Title, URL, Provider, Season, Episode, EpName populated.
// Replay loops internally; all other actions return to the caller.
func PlayWithControls(url, title, referer, userAgent string, subtitles []string, debug bool, startSecs float64, hctx HookContext) (PlayResult, error)
```

## Internal Helpers

| Function | Purpose |
|----------|---------|
| `buildPlayerCmd` | Constructs the platform-appropriate `*exec.Cmd`; appends `cfg.MpvArgs` for mpv |
| `checkAndroid` | Runs `uname -o` to detect the Android environment |
| `isMPV` | Returns true when the platform/config uses mpv (Linux/Windows/FreeBSD, player != vlc) |
| `generateSocketPath` | Returns a unique path for the mpv IPC socket via gopv |
| `connectMPVIPC` | Retries connecting to the mpv IPC socket for up to 3 seconds |
| `readPositionViaIPC` | Queries `time-pos` from a live gopv client; returns 0 on error |
