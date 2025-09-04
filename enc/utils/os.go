package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	//"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	//"sync"
	"syscall"
	"time"
)

// Enshell runs a shell command and returns the output and error
func Enshell(cmd string) (stdout string, stderr string, err error) {
	c := exec.Command("sh", "-c", cmd)
	var outBuf, errBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err = c.Run()
	return outBuf.String(), errBuf.String(), err
}

const (
	// max output size like your Python truncation threshold
	maxHTMLSize = 65536
)

// ChooseMediaInfoBinary decides which mediainfo binary to call.
//
// It prefers, in order:
//  1. env var MEDIATOOL (if set)
//  2. .bin/<os>-<arch>/mediainfo
//  3. .bin/mediainfo
func ChooseMediaInfoBinary() string {
	if v := os.Getenv("MEDIAINFOPATH"); v != "" {
		return v
	}
	// prefer per-platform folder, e.g. .bin/linux-amd64/mediainfo
	perPlatform := filepath.Join(".bin", runtime.GOOS+"-"+runtime.GOARCH, "mediainfo")
	if _, err := os.Stat(perPlatform); err == nil {
		return perPlatform
	}
	// fallback to .bin/mediainfo
	fallback := filepath.Join(".bin", "mediainfo")
	return fallback
}

// MediaInfoHTML runs mediainfo and returns HTML output.
//
// ctx: caller's context (used for timeout/cancel)
// file: path to the media file
// full: whether to request full output (adds --Full / -f)
func MediaInfoHTML(ctx context.Context, file string, full bool) (string, error) {
	bin := ChooseMediaInfoBinary()

	// ensure binary exists
	if info, err := os.Stat(bin); err != nil || info.IsDir() {
		return "", fmt.Errorf("mediainfo binary not found at %s", bin)
	}

	// build args
	args := []string{"--Output=HTML"}
	if full {
		// mediainfo supports -f/--Full; some builds accept "--Full" as well.
		// use the long form to be explicit:
		args = append(args, "--Full")
	}
	args = append(args, file)

	// give it a reasonable timeout to avoid hanging processes
	// if ctx already has a deadline, use it; otherwise wrap with 15s
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	} else {
		cancel = func() {}
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	output := string(out)

	// If process failed but produced output, return both
	if err != nil {
		// If no output at all, return the error
		if len(output) == 0 {
			return "", fmt.Errorf("mediainfo failed: %w", err)
		}
	}

	// Truncate to avoid huge payloads
	if len(output) > maxHTMLSize {
		// keep ~65430 bytes plus truncation notice (matches original).
		cut := 65430
		if cut > len(output) {
			cut = len(output)
		}
		output = output[:cut] + "<strong>...<strong><br><br><strong>(TRUNCATED DUE TO CONTENT EXCEEDING MAX LENGTH)<strong>"
	}

	return output, nil
}

func PostMediaInfo(path string) (string, error) {
	mInfo, err := MediaInfoHTML(context.Background(), path, true)
	if err != nil {
		return "", err
	}
	page, err2 := PostToTelegraph("MediaInfo", mInfo)
	if err2 != nil {
		return "", err2
	}
	return page.Url, nil
}

// IsRunning checks if a process is still running
func IsRunning(cmd *exec.Cmd) bool {
	return cmd.ProcessState == nil || !cmd.ProcessState.Exited()
}

// CheckExt returns a file path with ensured extension
func CheckExt(path, ext string, getSplit, override bool) (fullPath, root, finalExt string) {
	if ext == "" {
		ext = ".mkv"
	}
	root, ext_ := filepath.Split(path)
	if filepath.Ext(path) == "" || override {
		path = strings.TrimSuffix(path, filepath.Ext(path)) + ext
	} else {
		ext = ext_
	}
	if getSplit {
		return path, root, ext
	}
	return path, "", ""
}

// SRemove removes files or directories silently
func SRemove(folders bool, filenames ...string) {
	for _, name := range filenames {
		if folders {
			_ = os.RemoveAll(name)
		} else {
			_ = os.Remove(name)
		}
	}
}

// KillProcessByName kills a process by name (simplified; cross-platform limitations)
func KillProcessByName(name string) error {
	return fmt.Errorf("not implemented: KillProcessByName should be implemented with proper OS tools")
}

// QClean removes temp directories and attempts to kill ffmpeg
func QClean(ffmpegPath string) {
	paths := []string{"downloads", "downloads2", "encode", "mux", "minfo"}
	for _, p := range paths {
		_ = os.RemoveAll(p)
	}
	if _, err := os.Stat(ffmpegPath); err == nil {
		data, _ := os.ReadFile(ffmpegPath)
		ff := strings.Fields(string(data))
		if len(ff) > 0 {
			_ = KillProcessByName(ff[0])
		}
	}
}

// ReadNToLastLine reads the nth before last line
func ReadNToLastLine(filename string, n int) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if len(lines) < n {
		return "", fmt.Errorf("not enough lines")
	}
	return lines[len(lines)-n], nil
}

// FileExists checks if file exists
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DirExists checks if directory exists
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// SizeOf returns size of file in bytes
func SizeOf(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// XOr66 kills all user processes (DANGEROUS, placeholder)
func XOr66() {
	// Not safe or recommended to implement cross-platform
}

// ReX restarts the bot
func ReX(args ...string) error {
	cmd := append([]string{os.Args[0]}, args...)
	return syscall.Exec(os.Args[0], cmd, os.Environ())
}

// Placeholder function for Update logic
func Updater(versionPath string) error {
	// Implement version check and updater mechanism if needed
	return nil
}

// GetStreamDuration parses ffprobe output and returns video duration in seconds
func GetStreamDuration(file string) int {
	cmd := fmt.Sprintf(`ffprobe -hide_banner -show_streams -show_format -print_format json "%s"`, file)
	out, _, err := Enshell(cmd)
	if err != nil {
		return 0
	}
	type Format struct {
		Duration string `json:"duration"`
	}
	type FFProbeOutput struct {
		Format Format `json:"format"`
	}

	var parsed FFProbeOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		return 0
	}
	return int(f)
}

// GetVideoThumbnail captures a thumbnail from a video file
func GetVideoThumbnail(file, output string, withDur bool) (string, int, error) {
	duration := GetStreamDuration(file)
	if duration <= 0 {
		return "", 0, nil
	}
	seek := duration / 2
	cmd := fmt.Sprintf(`ffmpeg -hide_banner -loglevel error -ss %d -i "%s" -vf thumbnail -q:v 1 -frames:v 1 -threads %d "%s" -y`, seek, file, runtime.NumCPU()/2, output)
	_, _, err := Enshell(cmd)
	if err != nil {
		return "", duration, err
	}
	if !FileExists(output) {
		return "", duration, nil
	}
	if withDur {
		return output, duration, nil
	}
	return output, 0, nil
}

// GetStreamInfo extracts audio/subtitle language info from streams
func GetStreamInfo(file string) (audioLang, subLang string, err error) {
	cmd := fmt.Sprintf(`ffprobe -hide_banner -show_streams -print_format json "%s"`, file)
	out, _, err := Enshell(cmd)
	if err != nil {
		return "", "", err
	}
	var data struct {
		Streams []map[string]interface{} `json:"streams"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return "", "", err
	}
	aBuf := strings.Builder{}
	sBuf := strings.Builder{}
	for _, stream := range data.Streams {
		type_, ok := stream["codec_type"].(string)
		if !ok {
			continue
		}
		tags, _ := stream["tags"].(map[string]interface{})
		lang := "?"
		if tags != nil {
			if l, ok := tags["language"].(string); ok {
				lang = l
			}
		}
		switch type_ {
		case "audio":
			aBuf.WriteString(lang + "|")
		case "subtitle":
			sBuf.WriteString(lang + "|")
		}
	}
	return strings.TrimSuffix(aBuf.String(), "|"), strings.TrimSuffix(sBuf.String(), "|"), nil
}

// PosInStream returns positions of specified languages in audio/subtitle streams
func PosInStream(file, lang1, lang2, mode string) (aPos, sPos int) {
	aPos, sPos = -1, -1
	aLang, sLang, err := GetStreamInfo(file)
	if err != nil {
		return aPos, sPos
	}
	if aLang != "" {
		langs := strings.Split(aLang, "|")
		for i, l := range langs {
			if l == lang1 || l == lang2 {
				aPos = i
				break
			}
		}
	}
	if sLang != "" {
		langs := strings.Split(sLang, "|")
		for i, l := range langs {
			if l == lang1 || l == lang2 {
				sPos = i
				break
			}
		}
	}
	switch mode {
	case "a", "audio":
		return aPos, -1
	case "s", "sub":
		return -1, sPos
	}
	return aPos, sPos
}

// Replace current process with new executable at same path
// returns only on error
func ExecRestart(chatID int64, msgID int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%d:%d", chatID, msgID)
	argv := []string{filepath.Base(exe), "restart", id}
	env := os.Environ()
	return syscall.Exec(exe, argv, env)
}
