package startup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"enc/config"
	"enc/internal/logger"
)

// InstallAria2 downloads the correct aria2 static zip for the current platform,
// extracts the aria2c binary and places it into binDir (creating binDir if needed).
// If a binary already exists at binDir/aria2c (or aria2c.exe on Windows) the function
// returns nil (no-op).
func InstallAria2(ctx context.Context, binDir string) (string, error) {
	if config.Conf.NoExternalDL {
		return "", nil
	}
	// Map platform -> zip filename
	baseURL := "https://github.com/abcfy2/aria2-static-build/releases/download/1.37.0/"
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var zipName string
	var targetName string // final binary name (aria2c or aria2c.exe)
	switch {
	case goos == "linux" && goarch == "amd64":
		zipName = "aria2-x86_64-linux-musl_static.zip"
		targetName = "aria2c"
	case goos == "linux" && goarch == "arm":
		zipName = "aria2-arm-linux-musleabi_static.zip"
		targetName = "aria2c"
	case goos == "linux" && goarch == "arm64":
		zipName = "aria2-aarch64-linux-musl_static.zip"
		targetName = "aria2c"
	case goos == "windows" && goarch == "amd64":
		zipName = "aria2-x86_64-w64-mingw32_static.zip"
		targetName = "aria2c.exe"
	default:
		return "", fmt.Errorf("platform not supported: os=%s arch=%s", goos, goarch)
	}

	destPath := filepath.Join(binDir, targetName)

	// If file already exists, do nothing.
	if fi, err := os.Stat(destPath); err == nil {
		// simple check: file exists and size > 0
		if !fi.IsDir() && fi.Size() > 0 {
			return destPath, nil
		}
	}

	// Ensure binDir exists
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create bin dir: %w", err)
	}

	fullURL := baseURL + zipName

	// Quick check: HEAD to ensure remote exists
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("build HEAD request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("remote HEAD failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("remote file not available: %s (status %d)", fullURL, resp.StatusCode)
	}

	// Download zip to temp file
	tmpZip, err := os.CreateTemp("", "aria2-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp zip: %w", err)
	}
	tmpZipPath := tmpZip.Name()
	defer func() {
		_ = tmpZip.Close()
		_ = os.Remove(tmpZipPath)
	}()

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("build GET request: %w", err)
	}
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
		return "", fmt.Errorf("download failed: status %d", resp2.StatusCode)
	}

	_, err = io.Copy(tmpZip, resp2.Body)
	if err != nil {
		return "", fmt.Errorf("save zip: %w", err)
	}
	if err := tmpZip.Close(); err != nil {
		return "", fmt.Errorf("close temp zip: %w", err)
	}

	// Open zip and find the aria2c entry
	zr, err := zip.OpenReader(tmpZipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	var found *zip.File
	// Look for an entry named exactly "aria2c" or "aria2c.exe" or an entry whose base name matches.
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if base == targetName || strings.EqualFold(base, targetName) {
			found = f
			break
		}
	}
	// Fallback: find any file that starts with "aria2" (best-effort)
	if found == nil {
		for _, f := range zr.File {
			base := filepath.Base(f.Name)
			if strings.HasPrefix(strings.ToLower(base), "aria2") {
				found = f
				break
			}
		}
	}

	if found == nil {
		return "", errors.New("aria2 binary not found in zip")
	}

	// Extract found file to a temp file, then move into place
	rc, err := found.Open()
	if err != nil {
		return "", fmt.Errorf("open zip entry: %w", err)
	}
	defer func() { _ = rc.Close() }()

	tmpBin, err := os.CreateTemp("", "aria2-bin-*")
	if err != nil {
		return "", fmt.Errorf("create temp bin: %w", err)
	}
	tmpBinPath := tmpBin.Name()
	// ensure cleanup on error
	cleanupTmpBin := func() {
		_ = tmpBin.Close()
		_ = os.Remove(tmpBinPath)
	}
	defer cleanupTmpBin()

	if _, err := io.Copy(tmpBin, rc); err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}
	if err := tmpBin.Close(); err != nil {
		return "", fmt.Errorf("close tmp bin: %w", err)
	}

	// On Unix set executable bit; on Windows it's not necessary but harmless to skip
	if goos != "windows" {
		if err := os.Chmod(tmpBinPath, 0o755); err != nil {
			return "", fmt.Errorf("chmod tmp bin: %w", err)
		}
	}

	// Atomic move: try rename, otherwise copy+remove fallback
	if err := atomicMove(tmpBinPath, destPath); err != nil {
		return "", fmt.Errorf("install binary: %w", err)
	}

	// success: remove tmp zip (deferred) and tmp bin (moved already so tmp removed is noop)
	return destPath, nil
}

// InstallQbittorrentNox downloads the appropriate static qbittorrent-nox binary for
// the current platform and installs it into binDir (creating binDir if needed).
// If the target binary already exists (non-empty) this is a no-op.
func InstallQbittorrentNox(ctx context.Context, binDir string) (string, error) {
	if config.Conf.NoExternalDL {
		return "", nil
	}
	baseURL := "https://github.com/userdocs/qbittorrent-nox-static/releases/download/release-5.1.2_v2.0.11/"

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var fileName string
	targetName := "qbittorrent-nox"

	// map platform -> filename (as provided)
	switch {
	case goos == "linux" && goarch == "amd64":
		fileName = "x86-qbittorrent-nox"
	case goos == "linux" && goarch == "arm":
		fileName = "armhf-qbittorrent-nox"
	case goos == "linux" && goarch == "arm64":
		fileName = "aarch64-qbittorrent-nox"
	default:
		return "", fmt.Errorf("install qbittorrent-nox: platform not supported: os=%s arch=%s", goos, goarch)
	}

	destPath := filepath.Join(binDir, targetName)

	// If file already exists and seems valid, do nothing.
	if fi, err := os.Stat(destPath); err == nil {
		if !fi.IsDir() && fi.Size() > 0 {
			return destPath, nil
		}
	}

	// Ensure binDir exists
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create bin dir: %w", err)
	}

	fullURL := baseURL + fileName

	// HEAD to ensure remote exists
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("build HEAD request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("remote HEAD failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("remote file not available: %s (status %d)", fullURL, resp.StatusCode)
	}

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "qb-nox-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("build GET request: %w", err)
	}
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
		return "", fmt.Errorf("download failed: status %d", resp2.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp2.Body); err != nil {
		return "", fmt.Errorf("save binary: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	// make executable
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	// atomic move into place (reuses atomicMove helper)
	if err := atomicMove(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("install binary: %w", err)
	}

	return destPath, nil
}

// atomicMove tries to rename src -> dst. If rename fails (cross-device), it falls back to copy+remove.
func atomicMove(src, dst string) error {
	// ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir for dst: %w", err)
	}

	// attempt rename
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// fallback copy
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src for copy: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst for copy: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("copy data: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync dst: %w", err)
	}

	// preserve exec bit on unix if possible
	if runtime.GOOS != "windows" {
		if err := out.Chmod(0o755); err != nil {
			logger.Warn(err)
		}
	}

	// remove src
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove src after copy: %w", err)
	}
	return nil
}
