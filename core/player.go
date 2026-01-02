package core

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func Play(url, title, referer string, subtitles []string) error {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "darwin":
		args := []string{
			"--no-stdin",
			"--keep-running",
			fmt.Sprintf("--mpv-referrer=%s", referer),
			url,
			fmt.Sprintf("--mpv-force-media-title=Playing %s", title),
		}
		for _, sub := range subtitles {
			args = append(args, fmt.Sprintf("--mpv-sub-files=%s", sub))
		}
		cmd = exec.Command("iina", args...)
		
	default:
		args := []string{
			url,
			fmt.Sprintf("--referrer=%s", referer),
			fmt.Sprintf("--force-media-title=Playing %s", title),
		}
		for _, sub := range subtitles {
			args = append(args, fmt.Sprintf("--sub-file=%s", sub))
		}
		cmd = exec.Command("mpv", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	fmt.Printf("Starting player for %s...\n", title)
	return cmd.Run()
}
