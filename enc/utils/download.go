package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"enc/internal/logger"
)

// downloadToFile downloads the given URL and writes to outPath.
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
