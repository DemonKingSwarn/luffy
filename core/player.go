package core

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var mpv_executable string = "mpv"
var vlc_executable string = "vlc"

func checkAndroid() bool {
	cmd := exec.Command("uname", "-o")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "Android"
}

// buildPlayerCmd constructs the player command for the current platform/config.
// It does NOT start the process.
func buildPlayerCmd(url, title, referer, userAgent string, subtitles []string, debug bool) (*exec.Cmd, error) {
	if runtime.GOOS == "windows" {
		mpv_executable = "mpv.exe"
		vlc_executable = "vlc.exe"
	} else {
		mpv_executable = "mpv"
		vlc_executable = "vlc"
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

	default:
		cfg := LoadConfig()
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
			// Default to mpv
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
			cmd = exec.Command(mpv_executable, args...)
		}
	}

	return cmd, nil
}

// Play starts the player and blocks until it exits (original behaviour).
func Play(url, title, referer, userAgent string, subtitles []string, debug bool) error {
	cmd, err := buildPlayerCmd(url, title, referer, userAgent, subtitles, debug)
	if err != nil {
		return err
	}
	if cmd == nil {
		return fmt.Errorf("could not build player command")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if debug && len(subtitles) > 0 {
		fmt.Printf("Subtitles found: %d\n", len(subtitles))
	}

	fmt.Printf("Starting player for %s...\n", title)
	return cmd.Run()
}

// StartPlayer launches the player in the background with its output suppressed
// so the terminal stays available for the fzf control menu.
// Returns the running *exec.Cmd so the caller can wait on or kill it.
func StartPlayer(url, title, referer, userAgent string, subtitles []string, debug bool) (*exec.Cmd, error) {
	cmd, err := buildPlayerCmd(url, title, referer, userAgent, subtitles, debug)
	if err != nil {
		return nil, err
	}
	if cmd == nil {
		return nil, fmt.Errorf("could not build player command")
	}

	// Suppress player output so fzf can own the terminal.
	cmd.Stdout = nil
	cmd.Stderr = nil

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

// PlayWithControls starts the player in the background and shows an fzf menu
// with Replay / Next / Previous / Quit.  It kills the running player process
// before returning so the caller can act on the chosen action immediately.
// Returns the chosen PlaybackAction.
func PlayWithControls(url, title, referer, userAgent string, subtitles []string, debug bool) (PlaybackAction, error) {
	for {
		fmt.Printf("Starting player for %s...\n", title)
		cmd, err := StartPlayer(url, title, referer, userAgent, subtitles, debug)
		if err != nil {
			return PlaybackQuit, err
		}

		// Channel that signals when the player process exits on its own.
		done := make(chan struct{})
		go func() {
			cmd.Wait() //nolint:errcheck
			close(done)
		}()

		actions := []string{
			string(PlaybackNext),
			string(PlaybackPrevious),
			string(PlaybackReplay),
			string(PlaybackQuit),
		}

		chosen := SelectAction("Playback:", actions)

		// Kill the player if it is still running.
		select {
		case <-done:
			// already exited
		default:
			if cmd.Process != nil {
				cmd.Process.Kill() //nolint:errcheck
			}
			<-done // wait for the goroutine to finish
		}

		if chosen == "" || chosen == string(PlaybackQuit) {
			return PlaybackQuit, nil
		}
		if chosen == string(PlaybackReplay) {
			// loop back and restart the same episode
			continue
		}
		return PlaybackAction(chosen), nil
	}
}
