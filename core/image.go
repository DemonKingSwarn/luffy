package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

func GetCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(home, ".cache", "luffy")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}
	return cacheDir, nil
}

func DownloadPoster(url string, title string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("empty url")
	}

	cacheDir, err := GetCacheDir()
	if err != nil {
		return "", err
	}

	// Sanitize filename
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	safeTitle := re.ReplaceAllString(title, "_")

	// Create a safe filename
	filename := safeTitle + ".jpg" // Assuming jpg, but ideally check content-type or extension
	fullPath := filepath.Join(cacheDir, filename)

	// Check if already exists
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}

	req, err := NewRequest("GET", url)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	out, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return fullPath, nil
}

func CleanCache() error {
	cacheDir, err := GetCacheDir()
	if err != nil {
		return err
	}

	d, err := os.Open(cacheDir)
	if err != nil {
		return err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, name := range names {
		err = os.RemoveAll(filepath.Join(cacheDir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

func PreviewPoster(path string) error {
	cfg := LoadConfig()
	cmd := exec.Command("chafa", "-f", cfg.ImageBackend, path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
