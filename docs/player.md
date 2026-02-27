# Player

`core/player.go` abstracts video player invocation across all supported platforms. It offers three modes: a simple blocking call, a background launch with suppressed output, and a full playback control loop with an fzf menu.

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

### 2. Blocking Play (Play)

`Play` connects the player's stdout and stderr to the terminal and calls `cmd.Run()`, blocking until the player exits. This is used for the movie flow and single-episode play where no next/previous navigation is needed.

### 3. Background Launch (StartPlayer)

`StartPlayer` calls `cmd.Start()` instead of `cmd.Run()` and sets `cmd.Stdout = nil` and `cmd.Stderr = nil` so the player's output is discarded. This keeps the terminal free for the fzf control menu. Returns the running `*exec.Cmd` so the caller can wait on it or kill it.

### 4. Playback Control Loop (PlayWithControls)

`PlayWithControls` is the main entrypoint for series playback. It runs a loop:

1. Call `StartPlayer` to launch the player in the background.
2. Spawn a goroutine that waits for the player process to exit naturally and closes a `done` channel.
3. Call `SelectAction("Playback:", ...)` to show an fzf menu with four options:
   - **Next** — move to the next episode.
   - **Previous** — move to the previous episode.
   - **Replay** — restart the same episode (loop back).
   - **Quit** — exit.
4. When the user picks an action, kill the player if it is still running (select on the `done` channel to avoid a double-close panic), then return the chosen `PlaybackAction`.
5. If `Replay` is chosen, skip the return and restart the loop with the same URL.

The fzf menu appears immediately after the player launches, so the user can navigate episodes while content plays — or after it finishes naturally.

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
```

## Public API

```go
// Play launches the player and blocks until it exits.
// stdout/stderr are connected to the terminal.
func Play(url, title, referer, userAgent string, subtitles []string, debug bool) error

// StartPlayer launches the player in the background with output suppressed.
// Returns the running *exec.Cmd. The caller is responsible for cmd.Wait().
func StartPlayer(url, title, referer, userAgent string, subtitles []string, debug bool) (*exec.Cmd, error)

// PlayWithControls starts the player in the background and shows an fzf
// menu (Next / Previous / Replay / Quit). Kills the player before returning.
// Replay loops internally; all other actions return to the caller.
func PlayWithControls(url, title, referer, userAgent string, subtitles []string, debug bool) (PlaybackAction, error)
```

## Internal Helpers

| Function | Purpose |
|----------|---------|
| `buildPlayerCmd` | Constructs the platform-appropriate `*exec.Cmd` without starting it |
| `checkAndroid` | Runs `uname -o` to detect the Android environment |
