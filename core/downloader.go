package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type DownloadMetadata struct {
	Title       string
	Duration    string
	Filesize    int64
	Format      string
	Resolution  string
	Filename    string
	Destination string
}

func getDownloadMetadata(url, referer, userAgent string) (*DownloadMetadata, error) {
	args := []string{
		url,
		"--dump-single-json",
		"--skip-download",
		"--referer", referer,
		"--user-agent", userAgent,
		"--no-warnings",
	}

	cmd := exec.Command("yt-dlp", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}

	var info struct {
		Title      string `json:"title"`
		Duration   int64  `json:"duration"`
		Filesize   int64  `json:"filesize"`
		Format     string `json:"format"`
		Resolution string `json:"resolution"`
		Filename   string `json:"_filename"`
	}
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	duration := fmt.Sprintf("%d:%02d", info.Duration/60, info.Duration%60)
	if info.Duration/3600 > 0 {
		duration = fmt.Sprintf("%d:%02d:%02d", info.Duration/3600, (info.Duration%3600)/60, info.Duration%60)
	}

	return &DownloadMetadata{
		Title:       info.Title,
		Duration:    duration,
		Filesize:    info.Filesize,
		Format:      info.Format,
		Resolution:  info.Resolution,
		Filename:    info.Filename,
		Destination: info.Filename,
	}, nil
}

func getTerminalWidth() int {
	if runtime.GOOS == "windows" {
		return 80
	}
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 80
	}
	parts := strings.Split(string(out), " ")
	if len(parts) < 2 {
		return 80
	}
	width := 0
	fmt.Sscanf(parts[1], "%d", &width)
	if width == 0 {
		return 80
	}
	return width
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func displayDownloadTable(meta *DownloadMetadata) {
	termWidth := getTerminalWidth()
	if termWidth < 60 {
		termWidth = 60
	}

	maxValueLen := termWidth - 20
	if maxValueLen < 30 {
		maxValueLen = 30
	}

	fmt.Println("\nDownload Information:")
	fmt.Println(strings.Repeat("─", termWidth))

	fmt.Printf("%-12s %s\n", "Property", "Value")
	fmt.Println(strings.Repeat("─", termWidth))

	displayField("Title", meta.Title, maxValueLen)
	displayField("Duration", meta.Duration, maxValueLen)
	displayField("Size", formatSize(meta.Filesize), maxValueLen)
	displayField("Format", meta.Format, maxValueLen)
	displayField("Resolution", meta.Resolution, maxValueLen)
	displayField("Destination", meta.Destination, maxValueLen)

	fmt.Println(strings.Repeat("─", termWidth))
	fmt.Println()
}

func displayField(label, value string, maxLen int) {
	if len(value) > maxLen {
		value = truncate(value, maxLen)
	}
	fmt.Printf("%-12s %s\n", label, value)
}

func formatSize(bytes int64) string {
	if bytes == 0 {
		return "unknown"
	}
	const (
		MB = 1024 * 1024
		GB = 1024 * 1024 * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func Download(basePath, dlPath, name, url, referer, userAgent string, subtitles []string, debug bool) error {
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

	fmt.Println("[download] Fetching metadata...")
	meta, err := getDownloadMetadata(url, referer, userAgent)
	if err != nil {
		fmt.Printf("[warning] Could not fetch metadata: %v\n", err)
	} else {
		meta.Destination = outputTemplate
		displayDownloadTable(meta)
	}

	args := []string{
		url,
		"--no-skip-unavailable-fragments",
		"--fragment-retries", "infinite",
		"-N", "16",
		"-o", outputTemplate,
		"--referer", referer,
		"--user-agent", userAgent,
	}

	if debug {
		fmt.Printf("Downloading to %s...\n", outputTemplate)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("yt-dlp timed out after 30 minutes")
		}
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

			if debug {
				fmt.Printf("Downloading subtitle to %s...\n", subPath)
			}
			if err := downloadFile(subURL, subPath); err != nil {
				if debug {
					fmt.Printf("Failed to download subtitle: %v\n", err)
				}
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
