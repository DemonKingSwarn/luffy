package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diniamo/gopv"
)

var mpv_executable string = "mpv"
var vlc_executable string = "vlc"

func formatCommand(cmd *exec.Cmd) string {
	if cmd == nil {
		return ""
	}

	parts := make([]string, 0, 1+len(cmd.Args))
	for i, part := range cmd.Args {
		if i == 0 {
			parts = append(parts, quoteCommandArg(part))
			continue
		}
		parts = append(parts, quoteCommandArg(part))
	}

	return strings.Join(parts, " ")
}

func quoteCommandArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if strings.ContainsAny(arg, " \t\n\"") {
		return strconv.Quote(arg)
	}
	return arg
}

func debugValueSummary(label, value string) string {
	if len(value) <= 120 {
		return fmt.Sprintf("%s(len=%d): %s", label, len(value), value)
	}

	head := value[:80]
	tail := value[len(value)-30:]
	return fmt.Sprintf("%s(len=%d): %s ... %s", label, len(value), head, tail)
}

func checkAndroid() bool {
	cmd := exec.Command("uname", "-o")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "Android"
}

// isMPV returns true when the current platform/config uses mpv for playback.
func isMPV() bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" || checkAndroid() {
		return false
	}
	cfg := LoadConfig()
	return cfg.Player != "vlc"
}

// mpvIPCSocket holds the state for an active MPV IPC session.
type mpvIPCSession struct {
	socketPath  string
	client      *gopv.Client
	mu          sync.Mutex
	lastPosSecs float64
}

// generateSocketPath returns a unique path for the mpv IPC socket.
// On Windows, mpv uses named pipes; IPC is skipped there.
func generateSocketPath() (string, error) {
	path, err := gopv.GeneratePath()
	if err != nil {
		return "", fmt.Errorf("failed to generate IPC socket path: %w", err)
	}
	return path, nil
}

// buildPlayerCmd constructs the player command for the current platform/config.
// socketPath, if non-empty (and on a supported platform), adds --input-ipc-server
// so we can connect via gopv IPC to track position.
// startSecs, if > 0, tells mpv to seek to that position before playing.
// cfg, if non-nil, is used to append MpvArgs; if nil LoadConfig() is called.
// It does NOT start the process.
func buildPlayerCmd(url, title, referer, userAgent string, subtitles []string, debug bool, socketPath string, startSecs float64, cfg *Config) (*exec.Cmd, error) {
	if runtime.GOOS == "windows" {
		mpv_executable = "mpv.exe"
		vlc_executable = "vlc.exe"
	} else {
		mpv_executable = "mpv"
		vlc_executable = "vlc"
	}

	if cfg == nil {
		cfg = LoadConfig()
	}

	var cmd *exec.Cmd

	if checkAndroid() {
		if debug {
			fmt.Printf("Starting VLC on Android for %s...\n", title)
		}
		args := []string{
			"start",
			"--user", "0",
			"-a", "android.intent.action.VIEW",
			"-d", url,
			"-n", "org.videolan.vlc/org.videolan.vlc.gui.video.VideoPlayerActivity",
			"-e", "title", fmt.Sprintf("Playing %s", title),
		}
		if len(subtitles) > 0 {
			args = append(args, "--es", "subtitles_location", subtitles[0])
		}
		cmd = exec.Command("am", args...)
		return cmd, nil
	}

	switch runtime.GOOS {
	case "darwin":
		if cfg.Player == "mpv" {
			// Default to mpv.
			args := []string{
				url,
				fmt.Sprintf("--referrer=%s", referer),
				fmt.Sprintf("--user-agent=%s", userAgent),
				fmt.Sprintf("--force-media-title=Playing %s", title),
			}
			for _, sub := range subtitles {
				if sub != "" {
					args = append(args, fmt.Sprintf("--sub-file=%s", sub))
				}
			}
			// IPC socket for position tracking via gopv.
			if socketPath != "" {
				args = append(args, fmt.Sprintf("--input-ipc-server=%s", socketPath))
			}
			// Resume from saved position (seconds).
			if startSecs > 0 {
				args = append(args, fmt.Sprintf("--start=%s", strconv.FormatFloat(startSecs, 'f', 3, 64)))
			}
			if debug && runtime.GOOS == "windows" {
				args = append(args, "--force-window=immediate", fmt.Sprintf("--log-file=%s", filepath.Join(os.TempDir(), "luffy-mpv.log")))
			}
			// Append extra user-configured mpv args.
			args = append(args, cfg.MpvArgs...)
			cmd = exec.Command(mpv_executable, args...)
		} else {
			args := []string{
				"--no-stdin",
				"--keep-running",
				fmt.Sprintf("--mpv-referrer=%s", referer),
				fmt.Sprintf("--mpv-user-agent=%s", userAgent),
				url,
				fmt.Sprintf("--mpv-force-media-title=Playing %s", title),
			}
			for _, sub := range subtitles {
				args = append(args, fmt.Sprintf("--mpv-sub-files=%s", sub))
			}
			cmd = exec.Command("iina", args...)
		}
	default:
		if cfg.Player == "vlc" {
			args := []string{
				url,
				fmt.Sprintf("--http-referrer=%s", referer),
				fmt.Sprintf("--http-user-agent=%s", userAgent),
				fmt.Sprintf("--meta-title=Playing %s", title),
			}
			for _, sub := range subtitles {
				if sub != "" {
					if strings.HasPrefix(sub, "http://") || strings.HasPrefix(sub, "https://") {
						args = append(args, fmt.Sprintf("--input-slave=%s", sub))
					} else {
						args = append(args, fmt.Sprintf("--sub-file=%s", sub))
					}
				}
			}
			cmd = exec.Command(vlc_executable, args...)
		} else {
			// Default to mpv.
			args := []string{
				url,
				fmt.Sprintf("--referrer=%s", referer),
				fmt.Sprintf("--user-agent=%s", userAgent),
				fmt.Sprintf("--force-media-title=Playing %s", title),
			}
			for _, sub := range subtitles {
				if sub != "" {
					args = append(args, fmt.Sprintf("--sub-file=%s", sub))
				}
			}
			// IPC socket for position tracking via gopv.
			if socketPath != "" {
				args = append(args, fmt.Sprintf("--input-ipc-server=%s", socketPath))
			}
			// Resume from saved position (seconds).
			if startSecs > 0 {
				args = append(args, fmt.Sprintf("--start=%s", strconv.FormatFloat(startSecs, 'f', 3, 64)))
			}
			if debug && runtime.GOOS == "windows" {
				args = append(args, "--force-window=immediate", fmt.Sprintf("--log-file=%s", filepath.Join(os.TempDir(), "luffy-mpv.log")))
			}
			// Append extra user-configured mpv args.
			args = append(args, cfg.MpvArgs...)
			cmd = exec.Command(mpv_executable, args...)
		}
	}

	return cmd, nil
}

// connectMPVIPC tries to connect to the mpv IPC socket, retrying for up to
// 3 seconds to allow mpv time to start and create the socket.
// Returns nil if the connection could not be established.
func connectMPVIPC(socketPath string) *gopv.Client {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		client, err := gopv.Connect(socketPath, nil)
		if err == nil {
			return client
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// readPositionViaIPC queries the current time-pos property from mpv over IPC.
// Returns 0 on any error.
func readPositionViaIPC(client *gopv.Client) float64 {
	if client == nil {
		return 0
	}
	data, err := client.Request("get_property", "time-pos")
	if err != nil {
		return 0
	}
	switch v := data.(type) {
	case float64:
		return v
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return 0
}

// Play starts the player and blocks until it exits.
// hctx provides metadata for lifecycle hooks (on_play / on_exit).
// Returns the final playback position in seconds; 0 if not tracked.
func Play(url, title, referer, userAgent string, subtitles []string, debug bool, startSecs float64, hctx HookContext) (float64, error) {
	cfg := LoadConfig()

	hctx.StreamURL = url
	hctx.Action = "play"
	RunHook(cfg.Hooks.OnPlay, hctx, debug)

	socketPath := ""
	if isMPV() {
		var err error
		socketPath, err = generateSocketPath()
		if err != nil && debug {
			fmt.Printf("Warning: could not generate IPC socket path: %v\n", err)
		}
		if socketPath != "" {
			defer os.Remove(socketPath)
		}
	}

	cmd, err := buildPlayerCmd(url, title, referer, userAgent, subtitles, debug, socketPath, startSecs, cfg)
	if err != nil {
		return 0, err
	}
	if cmd == nil {
		return 0, fmt.Errorf("could not build player command")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if debug && len(subtitles) > 0 {
		fmt.Printf("Subtitles found: %d\n", len(subtitles))
	}
	if debug {
		fmt.Printf("Player headers: Referer=%s | User-Agent=%s\n", referer, userAgent)
		fmt.Println(debugValueSummary("Player URL", url))
		fmt.Printf("Player command: %s\n", formatCommand(cmd))
	}

	fmt.Printf("Starting player for %s...\n", title)

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start player: %w", err)
	}

	// Connect to IPC and track position in background.
	var lastPos float64
	if socketPath != "" {
		client := connectMPVIPC(socketPath)
		if client != nil {
			defer func() {
				defer func() { recover() }()
				client.Close()
			}()
			var mu sync.Mutex
			stop := make(chan struct{})
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-stop:
						return
					case <-ticker.C:
						pos := readPositionViaIPC(client)
						if pos > 0 {
							mu.Lock()
							lastPos = pos
							mu.Unlock()
						}
					}
				}
			}()
			cmd.Wait() //nolint:errcheck
			close(stop)
			// One final read after mpv exits.
			if pos := readPositionViaIPC(client); pos > 0 {
				mu.Lock()
				lastPos = pos
				mu.Unlock()
			}
			mu.Lock()
			pos := lastPos
			mu.Unlock()
			hctx.Position = pos
			RunHook(cfg.Hooks.OnExit, hctx, debug)
			return pos, nil
		}
	}

	cmd.Wait() //nolint:errcheck
	RunHook(cfg.Hooks.OnExit, hctx, debug)
	return 0, nil
}

// StartPlayer launches the player in the background with its output suppressed
// so the terminal stays available for the fzf control menu.
// socketPath, if non-empty, is the IPC socket path passed to mpv.
// startSecs, if > 0, tells mpv to seek to that position before playing.
// cfg is used to append MpvArgs; if nil, LoadConfig() is called internally.
// Returns the running *exec.Cmd so the caller can wait on or kill it.
func StartPlayer(url, title, referer, userAgent string, subtitles []string, debug bool, socketPath string, startSecs float64, cfg *Config) (*exec.Cmd, error) {
	cmd, err := buildPlayerCmd(url, title, referer, userAgent, subtitles, debug, socketPath, startSecs, cfg)
	if err != nil {
		return nil, err
	}
	if cmd == nil {
		return nil, fmt.Errorf("could not build player command")
	}

	// Suppress player output so fzf can own the terminal.
	if debug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("Player headers: Referer=%s | User-Agent=%s\n", referer, userAgent)
		fmt.Println(debugValueSummary("Player URL", url))
		if runtime.GOOS == "windows" {
			fmt.Printf("MPV log file: %s\n", filepath.Join(os.TempDir(), "luffy-mpv.log"))
		}
		fmt.Printf("Player command: %s\n", formatCommand(cmd))
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	if debug && len(subtitles) > 0 {
		fmt.Printf("Subtitles found: %d\n", len(subtitles))
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start player: %w", err)
	}
	return cmd, nil
}

// PlaybackAction represents what the user chose from the playback control menu.
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

// PlayWithControls starts the player in the background and shows an fzf menu
// with Replay / Next / Previous / Quit.  It kills the running player process
// before returning so the caller can act on the chosen action immediately.
// Position is tracked in real-time via MPV IPC (gopv), not via watch-later files.
// hctx provides metadata for lifecycle hooks (on_play / on_exit).
// Returns a PlayResult containing the chosen action and the last tracked position.
func PlayWithControls(url, title, referer, userAgent string, subtitles []string, debug bool, startSecs float64, hctx HookContext) (PlayResult, error) {
	cfg := LoadConfig()

	for {
		fmt.Printf("Starting player for %s...\n", title)

		hctx.StreamURL = url
		hctx.Action = "play"
		RunHook(cfg.Hooks.OnPlay, hctx, debug)

		socketPath := ""
		if isMPV() {
			var err error
			socketPath, err = generateSocketPath()
			if err != nil && debug {
				fmt.Printf("Warning: could not generate IPC socket path: %v\n", err)
			}
		}

		cmd, err := StartPlayer(url, title, referer, userAgent, subtitles, debug, socketPath, startSecs, cfg)
		if err != nil {
			if socketPath != "" {
				os.Remove(socketPath)
			}
			return PlayResult{Action: PlaybackQuit}, err
		}

		// Connect to IPC and keep a live position counter.
		var ipcClient *gopv.Client
		var posMu sync.Mutex
		var lastPos float64
		var ipcStop chan struct{}

		if socketPath != "" {
			ipcClient = connectMPVIPC(socketPath)
			if ipcClient != nil {
				ipcStop = make(chan struct{})
				go func() {
					ticker := time.NewTicker(time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-ipcStop:
							return
						case <-ticker.C:
							pos := readPositionViaIPC(ipcClient)
							if pos > 0 {
								posMu.Lock()
								lastPos = pos
								posMu.Unlock()
							}
						}
					}
				}()
			}
		}

		// Signal when the player exits on its own.
		done := make(chan struct{})
		go func() {
			cmd.Wait() //nolint:errcheck
			close(done)
		}()

		if debug {
			select {
			case <-done:
				return PlayResult{Action: PlaybackQuit}, fmt.Errorf("player exited immediately after launch")
			case <-time.After(1500 * time.Millisecond):
			}
		}

		chosen := SelectActionCtx("Playback:", []string{
			string(PlaybackNext),
			string(PlaybackPrevious),
			string(PlaybackReplay),
			string(PlaybackQuit),
		}, done)

		// Kill the player if it is still running.
		select {
		case <-done:
			// already exited naturally (e.g. user pressed q in mpv)
		default:
			if cmd.Process != nil {
				cmd.Process.Kill() //nolint:errcheck
			}
			<-done
		}

		// Stop the IPC polling goroutine and do a final position read.
		var finalSecs float64
		if ipcClient != nil {
			close(ipcStop)
			if pos := readPositionViaIPC(ipcClient); pos > 0 {
				posMu.Lock()
				lastPos = pos
				posMu.Unlock()
			}
			// gopv panics with "close of closed channel" if the mpv process
			// already exited and tore down the IPC socket; recover gracefully.
			func() {
				defer func() { recover() }()
				ipcClient.Close()
			}()
		}
		if socketPath != "" {
			posMu.Lock()
			finalSecs = lastPos
			posMu.Unlock()
			os.Remove(socketPath)
		}

		hctx.Position = finalSecs
		RunHook(cfg.Hooks.OnExit, hctx, debug)

		if chosen == "" || chosen == string(PlaybackQuit) {
			return PlayResult{Action: PlaybackQuit, PositionSecs: finalSecs}, nil
		}
		if chosen == string(PlaybackReplay) {
			startSecs = 0
			continue
		}
		return PlayResult{Action: PlaybackAction(chosen), PositionSecs: finalSecs}, nil
	}
}
