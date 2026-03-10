package startup

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"slices"

	//"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/ulikunitz/xz"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
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
	targetName := "aria2c" // final binary name (aria2c or aria2c.exe)
	switch {
	case goos == "linux" && goarch == "amd64":
		zipName = "aria2-x86_64-linux-musl_static.zip"
	case goos == "linux" && goarch == "arm":
		zipName = "aria2-arm-linux-musleabi_static.zip"
	case goos == "linux" && goarch == "arm64":
		zipName = "aria2-aarch64-linux-musl_static.zip"
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
	tmpZip := &utils.TempFile{}
	err = tmpZip.Create("-aria2.zip")
	if err != nil {
		return "", fmt.Errorf("create temp zip: %w", err)
	}
	tmpZipPath := tmpZip.Name
	defer func() {
		_ = tmpZip.Remove()
	}()
	if err := utils.DownloadFileWithProgress(ctx, fullURL, tmpZipPath); err != nil {
		return "", nil
	}
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Suffix = " Extracting binary from zip..."
	_ = s.Color("yellow")
	s.Start()
	defer s.Stop()
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

// InstallFFmpeg downloads appropriate FFmpeg archive for the current platform,
// extracts ffmpeg, ffprobe and ffplay (or the .exe variants on Windows) from the
// archive's bin/ folder and installs them into binDir (creating it if needed).
//
// Returns map[string]string with keys "ffmpeg", "ffprobe", "ffplay" mapping to installed paths.
func InstallFFmpeg(ctx context.Context, binDir string) (map[string]string, error) {
	// platform -> archive filename + isZip flag
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var archiveName string
	var isZip bool

	switch {
	case goos == "linux" && goarch == "amd64":
		archiveName = "ffmpeg-n8.0-latest-linux64-gpl-8.0.tar.xz"
	case goos == "linux" && goarch == "arm64":
		archiveName = "ffmpeg-n8.0-latest-linuxarm64-gpl-8.0.tar.xz"
	case goos == "windows" && goarch == "arm64":
		archiveName = "ffmpeg-n8.0-latest-winarm64-gpl-8.0.zip"
	case goos == "windows" && goarch == "amd64":
		archiveName = "ffmpeg-n8.0-latest-win64-gpl-8.0.zip"
	default:
		return nil, fmt.Errorf("platform not supported: os=%s arch=%s", goos, goarch)
	}

	baseURL := "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/"
	fullURL := baseURL + archiveName

	// ensure binDir exists
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, fmt.Errorf("create bin dir: %w", err)
	}

	// target names
	exeSuffix := ""
	if goos == "windows" {
		isZip = true
		exeSuffix = ".exe"
	}
	targets := []string{"ffmpeg" + exeSuffix, "ffprobe" + exeSuffix, "ffplay" + exeSuffix}

	// if all targets exist, return early
	allExist := true
	installed := make(map[string]string)
	for _, t := range targets {
		dest := filepath.Join(binDir, t)
		if fi, err := os.Stat(dest); err == nil && !fi.IsDir() && fi.Size() > 0 {
			installed[t] = dest
			continue
		}
		allExist = false
	}
	if allExist {
		return installed, nil
	}

	// HEAD to ensure remote exists
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build HEAD request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote HEAD failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("remote file not available: %s (status %d)", fullURL, resp.StatusCode)
	}

	// Download archive to temp file
	tmpArchive := &utils.TempFile{}
	err = tmpArchive.Create("-ffmpeg-archive")
	if err != nil {
		return nil, fmt.Errorf("create temp archive: %w", err)
	}
	tmpArchivePath := tmpArchive.Name
	defer func() {
		_ = tmpArchive.Remove()
	}()

	if err := utils.DownloadFileWithProgress(ctx, fullURL, tmpArchivePath); err != nil {
		return nil, err
	}
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Suffix = " Extracting ffmpeg binary from archive..."
	_ = s.Color("yellow")
	s.Start()
	defer s.Stop()

	// extract targets from archive
	if isZip {
		if err := extractFromZip(tmpArchivePath, binDir, targets); err != nil {
			return nil, fmt.Errorf("extract zip: %w", err)
		}
	} else {
		if err := extractFromTarXz(tmpArchivePath, binDir, targets); err != nil {
			return nil, fmt.Errorf("extract tar.xz: %w", err)
		}
	}

	// verify installation and return installed paths
	for _, t := range targets {
		dest := filepath.Join(binDir, t)
		if fi, err := os.Stat(dest); err == nil && !fi.IsDir() && fi.Size() > 0 {
			installed[t] = dest
		} else {
			return nil, fmt.Errorf("binary %s not found after extraction", t)
		}
	}
	return installed, nil
}

// extractFromZip extracts files whose base name equals any of targets and with '/bin/' in path.
func extractFromZip(zipPath, binDir string, targets []string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	targetSet := make(map[string]struct{})
	for _, t := range targets {
		targetSet[t] = struct{}{}
	}

	for _, f := range r.File {
		bn := filepath.Base(f.Name)
		// require that the entry resides in a bin/ folder somewhere and base matches
		if _, want := targetSet[bn]; !want {
			continue
		}
		if !strings.Contains(f.Name, "/bin/") && !strings.Contains(f.Name, `\bin\`) {
			// ignore entries not under bin dir
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		tmpFile, err := os.CreateTemp("", "ffmpeg-bin-*")
		if err != nil {
			_ = rc.Close()
			return err
		}
		tmpPath := tmpFile.Name()

		_, err = io.Copy(tmpFile, rc)
		_ = tmpFile.Close()
		_ = rc.Close()
		if err != nil {
			_ = os.Remove(tmpPath)
			return err
		}

		// set exec bit on non-windows
		if runtime.GOOS != "windows" {
			if err := os.Chmod(tmpPath, 0o755); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}
		}

		// atomic move into place
		dest := filepath.Join(binDir, bn)
		if err := atomicMove(tmpPath, dest); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}
	return nil
}

// extractFromTarXz extracts files whose base name equals any of targets and with '/bin/' in path.
// extractFromTarXzSimple extracts matching entries from a tar.xz archive in a single pass.
// It reads the archive once, copies matching files out to temp files inside binDir and atomicMoves them.
func extractFromTarXz(tarXzPath, binDir string, targets []string) error {
	// ensure dest dir exists
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("mkdir binDir: %w", err)
	}

	targetSet := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		targetSet[t] = struct{}{}
	}

	f, err := os.Open(tarXzPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	xzr, err := xz.NewReader(bufio.NewReader(f))
	if err != nil {
		return fmt.Errorf("xz reader: %w", err)
	}
	tr := tar.NewReader(xzr)

	buf := make([]byte, 1<<20) // 1 MiB buffer

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}
		bn := filepath.Base(hdr.Name)
		if _, want := targetSet[bn]; !want {
			// skip content efficiently
			if hdr.Size > 0 {
				if _, err := io.CopyBuffer(io.Discard, tr, buf); err != nil {
					return fmt.Errorf("skip entry %s: %w", hdr.Name, err)
				}
			}
			continue
		}
		if !strings.Contains(hdr.Name, "/bin/") {
			if hdr.Size > 0 {
				if _, err := io.CopyBuffer(io.Discard, tr, buf); err != nil {
					return fmt.Errorf("skip non-bin %s: %w", hdr.Name, err)
				}
			}
			continue
		}

		// write tmp in binDir to allow rename
		tmpFile, err := os.CreateTemp(binDir, "extract-*")
		if err != nil {
			return fmt.Errorf("create tmp file: %w", err)
		}
		tmpPath := tmpFile.Name()

		if _, err := io.CopyBuffer(tmpFile, tr, buf); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("copy entry %s: %w", hdr.Name, err)
		}
		if err := tmpFile.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("close tmp: %w", err)
		}
		if runtime.GOOS != "windows" {
			_ = os.Chmod(tmpPath, 0o755)
		}

		dest := filepath.Join(binDir, bn)
		if err := atomicMove(tmpPath, dest); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("atomic move %s -> %s: %w", tmpPath, dest, err)
		}
	}

	return nil
}

// InstallMediaInfo downloads the appropriate static mediainfo  binary for
// the current platform and installs it into binDir (creating binDir if needed).
// If the target binary already exists (non-empty) this is a no-op.
func InstallMediaInfo(ctx context.Context, binDir string) (string, error) {
	baseURL := "https://github.com/Nubuki-all/MediainfoCli-Static/releases/download/latest/"
	baseURLW := "https://mediaarea.net/download/binary/mediainfo/25.09/MediaInfo_CLI_25.09_"

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var fileName string
	targetName := "mediainfo"

	// map platform -> filename (as provided)
	switch {
	case goos == "linux" && goarch == "amd64":
		fileName = "mediainfo-linux-amd64"
	case goos == "linux" && goarch == "arm":
		fileName = "mediainfo-linux-armv7"
	case goos == "linux" && goarch == "arm64":
		fileName = "mediainfo-linux-arm64"
	case goos == "windows" && goarch == "arm64":
		fileName = "Windows_x64.zip"
		targetName += ".exe"
	case goos == "windows" && goarch == "amd64":
		fileName = "Windows_ARM64.zip"
		targetName += ".exe"

	default:
		return "", fmt.Errorf("install mediainfo: platform not supported: os=%s arch=%s", goos, goarch)
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
	dlPrefix := "-mediainfo"
	fullURL := baseURL + fileName
	if goos == "windows" {
		fullURL = baseURLW + fileName
		dlPrefix = "-mediainfo.zip"
	}

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
	tmpFile := &utils.TempFile{}
	err = tmpFile.Create(dlPrefix)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name
	defer func() {
		_ = tmpFile.Remove()
	}()
	if err := utils.DownloadFileWithProgress(ctx, fullURL, tmpPath); err != nil {
		return "", err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Suffix = " Finishing up..."
	if goos == "windows" {
		s.Suffix = "Extracting MediaInfo_CLI..."
	}
	_ = s.Color("yellow")
	s.Start()
	defer s.Stop()

	if goos == "windows" {
		r, err := zip.OpenReader(tmpPath)
		if err != nil {
			return "", err
		}
		defer func() { _ = r.Close() }()

		for _, f := range r.File {
			bn := filepath.Base(f.Name)
			if !slices.Contains([]string{"MediaInfo.exe", "LIBCURL.DLL"}, f.Name) {
				continue
			}

			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			tmpFile, err := os.CreateTemp("", "mediainfo-*")
			if err != nil {
				_ = rc.Close()
				return "", err
			}
			tmpPath := tmpFile.Name()

			_, err = io.Copy(tmpFile, rc)
			_ = tmpFile.Close()
			_ = rc.Close()
			if err != nil {
				_ = os.Remove(tmpPath)
				return "", err
			}

			// atomic move into place
			if bn == "MediaInfo.exe" {
				bn = "mediainfo.exe"
			}
			dest := filepath.Join(binDir, bn)
			if err := atomicMove(tmpPath, dest); err != nil {
				_ = os.Remove(tmpPath)
				return "", err
			}
			return dest, nil
		}
	}
	// make executable
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	if err := atomicMove(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("install binary: %w", err)
	}

	return destPath, nil
}

// InstallAnitomy downloads the appropriate static anitomy binary for
// the current platform and installs it into binDir (creating binDir if needed).
// If the target binary already exists (non-empty) this is a no-op.
func InstallAnitomy(ctx context.Context, binDir string) (string, error) {
	baseURL := "https://github.com/Nubuki-all/binary-builder/releases/download/anitomy/"
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var fileName string
	targetName := "anitomy"
	if goos == "windows" {
		targetName += ".exe"
	}

	switch {
	case goos == "linux" && goarch == "amd64":
		fileName = "anitomy-amd64"
	case goos == "linux" && goarch == "arm":
		fileName = "anitomy-arm32"
	case goos == "linux" && goarch == "arm64":
		fileName = "anitomy-arm64"
	case goos == "windows" && goarch == "amd64":
		fileName = "anitomy-x64.exe"
	case goos == "windows" && goarch == "386":
		fileName = "anitomy-x86.exe"
	case goos == "windows" && goarch == "arm64":
		fileName = "anitomy-arm64.exe"
	default:
		return "", fmt.Errorf("install anitomy: binaries not available for platform: os=%s arch=%s", goos, goarch)
	}

	destPath := filepath.Join(binDir, targetName)

	// If file already exists and seems valid, do nothing.
	if fi, err := os.Stat(destPath); err == nil {
		if !fi.IsDir() && fi.Size() > 0 {
			bot.AnitomyAvailable = true
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
	tmpFile := &utils.TempFile{}
	err = tmpFile.Create("-anitomy")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name
	defer func() {
		_ = tmpFile.Remove()
	}()
	if err := utils.DownloadFileWithProgress(ctx, fullURL, tmpPath); err != nil {
		return "", err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Suffix = " Finishing up..."
	_ = s.Color("yellow")
	s.Start()
	defer s.Stop()

	// make executable
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	if err := atomicMove(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("install binary: %w", err)
	}

	bot.AnitomyAvailable = true

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
	tmpFile := &utils.TempFile{}
	err = tmpFile.Create("-qbittorrent-nox")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name
	defer func() {
		_ = tmpFile.Remove()
	}()
	if err := utils.DownloadFileWithProgress(ctx, fullURL, tmpPath); err != nil {
		return "", err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Suffix = " Finishing up..."
	_ = s.Color("yellow")
	s.Start()
	defer s.Stop()

	// make executable
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	if err := atomicMove(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("install binary: %w", err)
	}

	return destPath, nil
}

// InstallYtdlp downloads the appropriate yt-dlp binary for
// the current platform and installs it into binDir (creating binDir if needed).
// If the target binary already exists (non-empty) this is a no-op.
func InstallYtdlp(ctx context.Context, binDir string) (string, error) {
	if config.Conf.NoExternalDL {
		return "", nil
	}
	baseURL := "https://github.com/yt-dlp/yt-dlp/releases/download/2026.03.03/"
	denoURL := "https://github.com/denoland/deno/releases/download/v2.6.3/"

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var denoFileName, fileName string
	targetName := "yt-dlp"
	denoTargetName := "deno"

	switch {
	case goos == "linux" && goarch == "amd64":
		fileName = "yt-dlp_linux"
		denoFileName = "deno-x86_64-unknown-linux-gnu.zip"
	case goos == "linux" && goarch == "arm64":
		fileName = "yt-dlp_linux_aarch64"
		denoFileName = "deno-aarch64-unknown-linux-gnu.zip"
	case goos == "windows" && goarch == "arm64":
		fileName = "yt-dlp_arm64.exe"
	case goos == "windows" && goarch == "amd64":
		fileName = "yt-dlp_x86.exe"
		denoFileName = "deno-x86_64-pc-windows-msvc.zip"

	default:
		return "", fmt.Errorf("install yt-dlp: platform not supported: os=%s arch=%s", goos, goarch)
	}
	if goos == "windows" {
		targetName += ".exe"
		denoTargetName += ".exe"
	}

	destPath := filepath.Join(binDir, targetName)
	denoDestPath := filepath.Join(binDir, denoTargetName)

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
	tmpFile := &utils.TempFile{}
	err = tmpFile.Create("-" + fileName)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name
	defer func() {
		_ = tmpFile.Remove()
	}()
	if err := utils.DownloadFileWithProgress(ctx, fullURL, tmpPath); err != nil {
		return "", err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Suffix = " Finishing up..."
	_ = s.Color("yellow")
	s.Start()
	defer s.Stop()

	// make executable
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	if err := atomicMove(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("install binary: %w", err)
	}
	if denoFileName == "" {
		return destPath, nil
	}
	s.Stop()
	fullURL = denoURL + denoFileName
	req, err = http.NewRequestWithContext(ctx, http.MethodHead, fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("build HEAD request: %w", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("remote HEAD failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("remote file not available: %s (status %d)", fullURL, resp.StatusCode)
	}

	// Download zip to temp file
	tmpZip := &utils.TempFile{}
	err = tmpZip.Create("-deno.zip")
	if err != nil {
		return "", fmt.Errorf("create temp zip: %w", err)
	}
	tmpZipPath := tmpZip.Name
	defer func() {
		_ = tmpZip.Remove()
	}()
	if err := utils.DownloadFileWithProgress(ctx, fullURL, tmpZipPath); err != nil {
		return "", nil
	}
	s.Start()
	zr, err := zip.OpenReader(tmpZipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = zr.Close() }()
	var found *zip.File
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if strings.HasPrefix(strings.ToLower(base), "deno") {
			found = f
			break
		}
	}
	if found == nil {
		return "", errors.New("deno binary not found in zip")
	}
	rc, err := found.Open()
	if err != nil {
		return "", fmt.Errorf("open zip entry: %w", err)
	}
	defer func() { _ = rc.Close() }()

	tmpBin, err := os.CreateTemp("", "deno-bin-*")
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
	if err := atomicMove(tmpBinPath, denoDestPath); err != nil {
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
