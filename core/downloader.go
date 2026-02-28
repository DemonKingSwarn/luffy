package core

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Download downloads a stream to disk using a native Go implementation.
// HLS streams (*.m3u8) are downloaded by fetching and concatenating segments
// concurrently. Other URLs are downloaded directly, using parallel byte-range
// requests when the server supports them and the file is large enough.
//
// Subtitle files (VTT/SRT) are downloaded alongside the video when provided.
func Download(basePath, dlPath, name, url, referer, userAgent string, subtitles []string, debug bool) error {
	if dlPath == "" {
		dlPath = filepath.Join(basePath, "Downloads", "luffy")
	} else {
		dlPath = filepath.Join(dlPath, "luffy")
	}
	if err := os.MkdirAll(dlPath, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	cleanName := sanitizeFilename(name)

	isHLS := strings.Contains(url, ".m3u8")

	// HLS streams are downloaded as raw MPEG-TS first, then remuxed to .mp4
	// via ffmpeg (stream copy — no re-encode). Direct files are saved as .mp4.
	ext := ".mp4"
	if isHLS {
		ext = ".ts"
	}

	outputPath := filepath.Join(dlPath, cleanName+ext)
	outputPath = ensureUnique(outputPath)

	fmt.Printf("[download] Saving to: %s\n", outputPath)

	// Download subtitles first so they are available even if the video fails.
	if len(subtitles) > 0 {
		for i, subURL := range subtitles {
			if subURL == "" {
				continue
			}
			subExt := ".vtt"
			if strings.HasSuffix(subURL, ".srt") {
				subExt = ".srt"
			}
			subPath := filepath.Join(dlPath, cleanName)
			if i > 0 {
				subPath += fmt.Sprintf(".eng%d%s", i, subExt)
			} else {
				subPath += ".eng" + subExt
			}
			if debug {
				fmt.Printf("[download] Downloading subtitle to %s...\n", subPath)
			}
			if subErr := downloadFileWithRetry(subURL, subPath, 3); subErr != nil {
				fmt.Printf("[warning] Failed to download subtitle: %v\n", subErr)
			}
		}
	}

	ctx := context.Background()

	headers := map[string]string{
		"User-Agent": userAgent,
	}
	if referer != "" {
		headers["Referer"] = referer
	}

	var err error
	if isHLS {
		err = downloadHLSWithProgress(ctx, url, outputPath, headers, debug)
	} else {
		err = downloadDirect(ctx, url, outputPath, headers, debug)
	}
	if err != nil {
		// Rename to .partial so the user keeps what was downloaded and can
		// inspect/resume manually, instead of losing the file entirely.
		partialPath := outputPath + ".partial"
		if renErr := os.Rename(outputPath, partialPath); renErr == nil {
			fmt.Printf("[download] Partial file kept at: %s\n", partialPath)
		} else {
			_ = os.Remove(outputPath)
		}
		return fmt.Errorf("download failed: %w", err)
	}

	// Remux the raw MPEG-TS to .mp4 using ffmpeg (stream copy, no re-encode).
	if isHLS {
		mp4Path := strings.TrimSuffix(outputPath, ".ts") + ".mp4"
		// ensureUnique for the .mp4 in case it already exists too.
		mp4Path = ensureUnique(mp4Path)
		fmt.Printf("[download] Remuxing to mp4: %s\n", mp4Path)
		if ffErr := remuxToMP4(outputPath, mp4Path, debug); ffErr != nil {
			fmt.Printf("[warning] ffmpeg remux failed (%v); keeping .ts file\n", ffErr)
		} else {
			_ = os.Remove(outputPath) // remove the intermediate .ts
			outputPath = mp4Path
		}
	}

	fmt.Println("[download] Complete!")
	return nil
}

// sanitizeFilename replaces characters that are problematic in filenames.
func sanitizeFilename(name string) string {
	r := strings.NewReplacer(
		" ", "-",
		"\"", "",
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"<", "",
		">", "",
		"|", "-",
	)
	return r.Replace(name)
}

// ensureUnique appends a counter to the path stem until it finds a name that
// does not exist on disk.
func ensureUnique(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// getTerminalWidth returns the terminal column width, defaulting to 80.
func getTerminalWidth() int {
	if runtime.GOOS == "windows" {
		return 80
	}
	// Use os.Stdout fd to query terminal size via ioctl would require cgo;
	// fall back to a simple stty call on Unix-likes.
	return 80
}

// downloadHLSWithProgress downloads an HLS stream and prints segment progress.
// Once progress reaches 99.3% the download context is cancelled — the last
// handful of segments are unnecessary because ffmpeg can recover from the
// partial MPEG-TS stream via stream-copy.
func downloadHLSWithProgress(ctx context.Context, url, output string, headers map[string]string, debug bool) error {
	referer := headers["Referer"]

	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	printProgress := func(downloaded, total int) {
		if total == 0 {
			return
		}
		pct := float64(downloaded) / float64(total) * 100.0
		bar := progressBar(downloaded, total, 30)
		fmt.Printf("\r[download] %s %5.1f%% (%d/%d segments)", bar, pct, downloaded, total)
		// Stop fetching new segments once we are past 99.3%.
		if pct >= 99.3 {
			cancel()
		}
	}

	err := DownloadHLS(dlCtx, url, output, referer, printProgress)
	// Treat context-cancelled-by-us (>=99.3%) as success.
	if err != nil && dlCtx.Err() != nil {
		err = nil
	}
	bar := progressBar(1, 1, 30)
	fmt.Printf("\r[download] %s 100.0%%\n", bar)
	return err
}

// remuxToMP4 runs ffmpeg to stream-copy src (.ts) into dst (.mp4).
func remuxToMP4(src, dst string, debug bool) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	args := []string{"-i", src, "-c", "copy", dst}
	if !debug {
		// Suppress ffmpeg banner and stats output when not debugging.
		args = append([]string{"-hide_banner", "-loglevel", "error"}, args...)
	}
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// downloadDirect downloads a regular (non-HLS) URL.
// It attempts concurrent byte-range downloads for large files; falls back to
// a single-connection stream otherwise.
func downloadDirect(ctx context.Context, url, output string, headers map[string]string, debug bool) error {
	// HEAD request to check range support and content length.
	headReq, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return downloadDirectSingle(ctx, url, output, headers, debug)
	}
	for k, v := range headers {
		headReq.Header.Set(k, v)
	}

	headClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := headClient.Do(headReq)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return downloadDirectSingle(ctx, url, output, headers, debug)
	}
	_ = resp.Body.Close()

	contentLength := resp.ContentLength
	acceptRanges := resp.Header.Get("Accept-Ranges")

	// Use parallel download when ranges are supported and file is >10 MB.
	if contentLength > 10*1024*1024 && acceptRanges == "bytes" {
		if debug {
			fmt.Printf("[download] Using parallel download (%d bytes)\n", contentLength)
		}
		return downloadDirectConcurrent(ctx, url, output, headers, contentLength, debug)
	}

	return downloadDirectSingle(ctx, url, output, headers, debug)
}

// downloadDirectConcurrent downloads url in numParts parallel byte-range chunks.
// Each part is retried independently on failure so a single network blip near
// the end does not abort the entire download.
func downloadDirectConcurrent(ctx context.Context, url, output string, headers map[string]string, totalBytes int64, debug bool) error {
	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Pre-allocate.
	if err := f.Truncate(totalBytes); err != nil {
		return fmt.Errorf("failed to allocate file: %w", err)
	}

	const numParts = 8
	const maxPartRetries = 6
	partSize := totalBytes / numParts

	var wg sync.WaitGroup
	errChan := make(chan error, numParts)
	var downloadedBytes int64

	// Progress monitor goroutine.
	monitorDone := make(chan struct{})
	monitorCtx, monitorCancel := context.WithCancel(ctx)
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
				current := atomic.LoadInt64(&downloadedBytes)
				pct := float64(current) / float64(totalBytes) * 100.0
				bar := progressBar(int(current), int(totalBytes), 30)
				fmt.Printf("\r[download] %s %5.1f%%", bar, pct)
			}
		}
	}()

	for i := 0; i < numParts; i++ {
		start := int64(i) * partSize
		end := start + partSize - 1
		if i == numParts-1 {
			end = totalBytes - 1
		}

		wg.Add(1)
		go func(partIdx int, start, end int64) {
			defer wg.Done()

			var lastErr error
			for attempt := 0; attempt <= maxPartRetries; attempt++ {
				if attempt > 0 {
					// Exponential backoff with jitter.
					base := time.Duration(1<<uint(attempt-1)) * time.Second
					if base > 30*time.Second {
						base = 30 * time.Second
					}
					jitter := time.Duration(rand.Int63n(int64(base) / 4))
					sleep := base + jitter
					select {
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					case <-time.After(sleep):
					}
				}

				written, err := downloadPart(ctx, url, f, headers, start, end, &downloadedBytes, attempt > 0)
				if err == nil {
					return
				}
				lastErr = err

				// Rewind progress counter and restart from where we left off.
				start += written

				select {
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				default:
				}

				if debug {
					fmt.Printf("\n[download] Part %d attempt %d failed (%v), retrying...\n", partIdx, attempt+1, err)
				}
			}
			errChan <- fmt.Errorf("part %d failed after %d attempts: %w", partIdx, maxPartRetries+1, lastErr)
		}(i, start, end)
	}

	wg.Wait()
	monitorCancel()
	<-monitorDone

	// Print final 100% bar.
	bar := progressBar(1, 1, 30)
	fmt.Printf("\r[download] %s 100.0%%\n", bar)

	close(errChan)
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

// downloadPart fetches bytes [start, end] from url into f at the correct
// offset. It returns the number of bytes successfully written so the caller
// can advance the start offset on retry.
// When resuming is true the progress counter is not double-counted.
func downloadPart(ctx context.Context, url string, f *os.File, headers map[string]string, start, end int64, downloadedBytes *int64, resuming bool) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	buf := make([]byte, 32*1024)
	offset := start
	var written int64
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.WriteAt(buf[:n], offset); wErr != nil {
				return written, wErr
			}
			offset += int64(n)
			written += int64(n)
			atomic.AddInt64(downloadedBytes, int64(n))
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return written, err
		}
	}
	return written, nil
}

// downloadDirectSingle downloads url using a single HTTP connection, streaming
// directly to the output file. On a mid-stream connection drop it resumes from
// where it left off using a Range request (if the server supports it).
func downloadDirectSingle(ctx context.Context, url, output string, headers map[string]string, debug bool) error {
	const maxRetries = 6

	out, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = out.Close() }()

	var totalBytes int64 = -1 // -1 = unknown
	var downloaded int64
	lastReport := time.Now()

	client := &http.Client{Timeout: 0}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			base := time.Duration(1<<uint(attempt-1)) * time.Second
			if base > 30*time.Second {
				base = 30 * time.Second
			}
			jitter := time.Duration(rand.Int63n(int64(base) / 4))
			sleep := base + jitter
			if debug {
				fmt.Printf("\n[download] Connection lost at %s, retrying in %v (attempt %d/%d)...\n",
					formatSize(downloaded), sleep, attempt+1, maxRetries)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
		}

		// Request only the remaining bytes when resuming.
		if downloaded > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
		}

		resp, err := client.Do(req)
		if err != nil {
			continue // retry
		}

		// Accept 200 (first attempt or server doesn't support ranges) or 206.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}
			continue
		}

		// Capture total length from the first successful response.
		if totalBytes < 0 {
			totalBytes = resp.ContentLength
		}

		// If the server responded 200 to a range request (doesn't support
		// ranges), seek the file back to 0 and restart.
		if downloaded > 0 && resp.StatusCode == http.StatusOK {
			if _, sErr := out.Seek(0, io.SeekStart); sErr != nil {
				_ = resp.Body.Close()
				return fmt.Errorf("failed to seek output file: %w", sErr)
			}
			downloaded = 0
		}

		buf := make([]byte, 32*1024)
		var readErr error
		for {
			select {
			case <-ctx.Done():
				_ = resp.Body.Close()
				return ctx.Err()
			default:
			}

			n, rErr := resp.Body.Read(buf)
			if n > 0 {
				if _, wErr := out.Write(buf[:n]); wErr != nil {
					_ = resp.Body.Close()
					return fmt.Errorf("failed to write to file: %w", wErr)
				}
				downloaded += int64(n)

				if time.Since(lastReport) >= 500*time.Millisecond {
					if totalBytes > 0 {
						bar := progressBar(int(downloaded), int(totalBytes), 30)
						pct := float64(downloaded) / float64(totalBytes) * 100.0
						fmt.Printf("\r[download] %s %5.1f%% (%s / %s)",
							bar, pct, formatSize(downloaded), formatSize(totalBytes))
					} else {
						fmt.Printf("\r[download] %s downloaded", formatSize(downloaded))
					}
					lastReport = time.Now()
				}
			}

			if rErr != nil {
				if rErr == io.EOF {
					// Successful completion.
					_ = resp.Body.Close()
					readErr = nil
				} else {
					_ = resp.Body.Close()
					readErr = rErr
				}
				break
			}
		}

		if readErr == nil {
			break // done
		}
		// Otherwise loop and retry from 'downloaded' offset.
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("error closing output file: %w", err)
	}

	if totalBytes > 0 {
		bar := progressBar(int(downloaded), int(totalBytes), 30)
		fmt.Printf("\r[download] %s 100.0%% (%s)", bar, formatSize(downloaded))
	}
	fmt.Println()
	return nil
}

// progressBar returns a simple ASCII progress bar string.
func progressBar(done, total, width int) string {
	if total == 0 {
		return "[" + strings.Repeat(" ", width) + "]"
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}

// formatSize returns a human-readable byte count.
func formatSize(n int64) string {
	const (
		MB = 1024 * 1024
		GB = 1024 * 1024 * 1024
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.2f MB", float64(n)/float64(MB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// downloadFileWithRetry downloads a single file with up to maxRetries attempts.
func downloadFileWithRetry(url, path string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := downloadFile(url, path); err == nil {
			return nil
		} else {
			lastErr = err
		}
		_ = os.Remove(path)
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return lastErr
}

// downloadFile downloads a single URL to path using a plain GET request.
func downloadFile(url, path string) error {
	client := NewClient()
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}
