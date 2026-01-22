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

func Download(basePath, dlPath, name, url, referer, userAgent string, subtitles []string) error {
	if dlPath == "" {
		dlPath = filepath.Join(basePath, "Downloads", "luffy")
	} else {
		dlPath = filepath.Join(dlPath, "luffy")
	}
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
		"--user-agent", userAgent,
	}

	fmt.Printf("Downloading to %s...\n", outputTemplate)
	cmd := exec.Command("yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp failed: %w", err)
	}

	if len(subtitles) > 0 {
		for i, subURL := range subtitles {
			ext := ".vtt"
			if strings.HasSuffix(subURL, ".srt") {
				ext = ".srt"
			}

			subPath := filepath.Join(dlPath, cleanName)
			if i > 0 {
				subPath += fmt.Sprintf(".eng%d%s", i, ext)
			} else {
				subPath += ".eng" + ext
			}

			fmt.Printf("Downloading subtitle to %s...\n", subPath)
			if err := downloadFile(subURL, subPath); err != nil {
				fmt.Printf("Failed to download subtitle: %v\n", err)
			}
		}
	}

	fmt.Println("Download complete!")
	return nil
}

func downloadFile(url, filepath string) error {
	client := NewClient()
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
