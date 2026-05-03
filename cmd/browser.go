package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/demonkingswarn/luffy/core/providers"
)

func getVidsrcURL(title string, isSeries bool, client *http.Client, debug bool) (string, error) {
	vidsrc := providers.NewVidsrc(client)
	results, err := vidsrc.Search(title)
	if err != nil {
		return "", fmt.Errorf("vidsrc search failed: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no results from vidsrc for: %s", title)
	}
	firstURL := results[0].URL
	if debug {
		fmt.Printf("Vidsrc found: %s -> %s\n", results[0].Title, firstURL)
	}
	mediaID, err := vidsrc.GetMediaID(firstURL)
	if err != nil {
		return "", fmt.Errorf("failed to get vidsrc media ID: %w", err)
	}
	link, err := vidsrc.GetLink(mediaID)
	if err != nil {
		return "", fmt.Errorf("failed to get vidsrc link: %w", err)
	}
	return link, nil
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}