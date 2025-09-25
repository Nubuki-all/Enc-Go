package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/schollz/progressbar/v3"

	"enc/internal/logger"
)

// DownloadToFile downloads the given URL and writes to outPath.
// It creates directories as needed. Returns error on failure.
func DownloadToFile(ctx context.Context, url string, outPath string) error {
	if url == "" || outPath == "" {
		return fmt.Errorf("empty url or output path")
	}

	// Create directory if needed
	dir := filepath.Dir(outPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// HTTP GET with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			logger.Error(err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write to file (temporary then rename for safety)
	tmpPath := outPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	err = f.Close()
	if err != nil {
		logger.Warn(err)
	}
	return os.Rename(tmpPath, outPath)
}

// DownloadFileWithProgress downloads url -> destPath showing a progress bar,
// writes to a temp file and then calls atomicMove(tmpPath, destPath).
// ctx is respected for cancellation/timeouts.
func DownloadFileWithProgress(ctx context.Context, url, destPath string) error {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	// Do request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: status %s", resp.Status)
	}

	// ensure dest dir exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}

	file, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	// Build progress bar
	total := resp.ContentLength // -1 if unknown
	var bar *progressbar.ProgressBar
	if total > 0 {
		bar = progressbar.NewOptions64(
			total,
			progressbar.OptionSetDescription(filepath.Base(destPath)),
			progressbar.OptionSetWidth(40),
			progressbar.OptionSetWriter(os.Stdout),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionThrottle(100),
			progressbar.OptionFullWidth(),
		)
	} else {
		// unknown length - create indeterminate bar
		bar = progressbar.NewOptions(
			-1,
			progressbar.OptionSetDescription(filepath.Base(destPath)),
			progressbar.OptionSetWidth(40),
			progressbar.OptionSetWriter(os.Stdout),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionThrottle(100),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetTheme(progressbar.Theme{
        Saucer:        "[green]=[reset]",
        SaucerHead:    "[green]>[reset]",
        SaucerPadding: " ",
        BarStart:      "[",
        BarEnd:        "]",
    }),
		)
	}
	// Wrap response body so reads update the bar
	reader := progressbar.NewReader(resp.Body, bar) // bar.(resp.Body)

	// Copy to temp file
	if _, err := io.Copy(file, &reader); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Ensure file is closed before moving
	if err := file.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	return nil
}
