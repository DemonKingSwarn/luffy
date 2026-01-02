package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Download(basePath, name, url, referer string, subtitles []string) error {
	dlPath := filepath.Join(basePath, "luffy-downloads")
	if err := os.MkdirAll(dlPath, 0755); err != nil {
		return err
	}

	cleanName := strings.ReplaceAll(name, " ", "-")
	cleanName = strings.ReplaceAll(cleanName, "\"", "")
	
	outputTemplate := filepath.Join(dlPath, cleanName+".mp4")

	args := []string{
		url,
		"--no-skip-unavailable-fragments",
		"--fragment-retries", "infinite",
		"-N", "16",
		"-o", outputTemplate,
		"--referer", referer,
	}

	fmt.Printf("Downloading to %s...\n", outputTemplate)
	cmd := exec.Command("yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp failed: %w", err)
	}

	if len(subtitles) > 0 {
		subURL := subtitles[0]
		subPath := filepath.Join(dlPath, cleanName+".vtt")
		
		fmt.Printf("Downloading subtitle to %s...\n", subPath)
		if err := downloadFile(subURL, subPath); err != nil {
			fmt.Printf("Failed to download subtitle: %v\n", err)
		}
	}

	fmt.Println("Download complete!")
	return nil
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
