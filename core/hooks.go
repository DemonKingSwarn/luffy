package core

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

// HookContext carries the metadata exposed to every hook script via environment
// variables.  Zero values are fine — the corresponding env vars will be set to
// empty strings / "0".
type HookContext struct {
	Title     string
	URL       string
	Season    int
	Episode   int
	EpName    string
	Provider  string
	Action    string  // "play" or "download"
	StreamURL string  // resolved stream URL (empty for on_exit before play)
	Position  float64 // playback position in seconds (on_exit only)
}

// hookEnv converts a HookContext into the slice of KEY=VALUE pairs that will
// be appended to the hook process's environment.
func hookEnv(hctx HookContext) []string {
	return []string{
		"LUFFY_TITLE=" + hctx.Title,
		"LUFFY_URL=" + hctx.URL,
		"LUFFY_SEASON=" + strconv.Itoa(hctx.Season),
		"LUFFY_EPISODE=" + strconv.Itoa(hctx.Episode),
		"LUFFY_EP_NAME=" + hctx.EpName,
		"LUFFY_PROVIDER=" + hctx.Provider,
		"LUFFY_ACTION=" + hctx.Action,
		"LUFFY_STREAM_URL=" + hctx.StreamURL,
		"LUFFY_POSITION=" + strconv.FormatFloat(hctx.Position, 'f', 3, 64),
	}
}

// RunHook executes the hook command string in a shell, blocking until it
// completes.  If command is empty the call is a no-op.  Errors are printed
// but do not abort the caller — hooks are best-effort.
func RunHook(command string, hctx HookContext, debug bool) {
	if command == "" {
		return
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	// Inherit the current environment and append LUFFY_* vars so scripts can
	// also access PATH, HOME, etc.
	cmd.Env = append(os.Environ(), hookEnv(hctx)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if debug {
		fmt.Printf("[hook] running: %s\n", command)
	}

	if err := cmd.Run(); err != nil {
		fmt.Printf("[hook] command failed: %v\n", err)
	}
}
