package core

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func checkAndroid() bool {
	cmd := exec.Command("uname", "-o")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "Android"
}

func Play(url, title, referer string, subtitles []string) error {
	var cmd *exec.Cmd

	if checkAndroid() {
		fmt.Println("~ Android Detected ~")
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
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Printf("Starting VLC on Android for %s...\n", title)
		return cmd.Run()
	}

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
