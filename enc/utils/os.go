package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

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

	"enc/config"
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

// MediaInfoText runs mediainfo and returns text output.
//
// ctx: caller's context
// file: path to the media file
// full: whether to request full output
func MediaInfoText(ctx context.Context, file string, full bool) (string, error) {
	// build args
	var args []string
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

	cmd := exec.CommandContext(ctx, "mediainfo", args...)
	out, err := cmd.CombinedOutput()
	output := string(out)

	// If process failed but produced output, return both
	if err != nil {
		// If no output at all, return the error
		if len(output) == 0 {
			return "", fmt.Errorf("mediainfo failed: %w", err)
		}
	}
	return output, nil
}

func PostMediaInfo(path string) (string, error) {
	mInfo, err := MediaInfoText(context.Background(), path, false)
	if err != nil {
		return "", err
	}
	page, err2 := PostToTelegraph("MediaInfo", ParseMediaInfoTextToHTML(mInfo))
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

// GetVideoThumbnailAndDuration captures a thumbnail from a video file
func GetVideoThumbnailAndDuration(file, output string) (string, int, error) {
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
	return output, duration, nil
}

// GetStreamInfo extracts audio/subtitle language info from streams
func GetStreamInfo(file string) (audioLang, subLang string, err error) {
	cmd := fmt.Sprintf(`ffprobe -hide_banner -show_streams -print_format json "%s"`, file)
	out, _, err := Enshell(cmd)
	if err != nil {
		return "", "", err
	}
	var data struct {
		Streams []map[string]any `json:"streams"`
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
		tags, _ := stream["tags"].(map[string]any)
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

// Restart current process with new executable at same path
// returns only on error
func Restart(chatID int64, msgID int) error {
	err := RestartDetached(chatID, msgID)
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

func RestartDetached(chatID int64, msgID int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%d:%d", chatID, msgID)
	argv := []string{"restart", id}
	// env := os.Environ()
	cmd := exec.Command(exe, argv...)
	devnull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = nil
	} else {
		defer devnull.Close() //nolint:errcheck
		cmd.Stdin = devnull
		cmd.Stdout = devnull
		cmd.Stderr = devnull
	}

	// Detach: create a new session so the child won't be in the parent's process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	// Optionally close inherited fds by setting cmd.ExtraFiles = nil and redirect streams.
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

type TempFile struct {
	File     *os.File
	Name     string
	KeepOpen bool
}

func (tmp *TempFile) Create(ext string) (err error) {
	dir, err := filepath.Abs(config.Conf.TempDir)
	if err != nil {
		return err
	}
	tmp.File, err = os.CreateTemp(dir, "*"+ext)
	if err != nil {
		return err
	}
	tmp.Name = tmp.File.Name()
	if !tmp.KeepOpen {
		if err = tmp.File.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (tmp *TempFile) FromBytes(ext string, bytes []byte) (err error) {
	dir, err := filepath.Abs(config.Conf.TempDir)
	if err != nil {
		return err
	}
	tmp.File, err = os.CreateTemp(dir, "*"+ext)
	if err != nil {
		return err
	}
	tmp.Name = tmp.File.Name()
	if _, err = tmp.File.Write(bytes); err != nil {
		return err
	}
	if !tmp.KeepOpen {
		if err = tmp.File.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (tmp *TempFile) Remove() (err error) {
	if tmp.File == nil {
		return nil
	}
	if tmp.KeepOpen {
		if err = tmp.File.Close(); err != nil {
			return err
		}
	}
	return os.Remove(tmp.Name)
}

var reDup = regexp.MustCompile(`-(\d+)$`)

func RenameDup(fileName string) string {
	if fileName == "" {
		return ""
	}

	ext := filepath.Ext(fileName)
	baseName := fileName[:len(fileName)-len(ext)]

	// Find if there is already a "-#", e.g., "video-1"
	match := reDup.FindStringSubmatch(baseName)
	if match != nil {
		// Increment the existing number
		num, _ := strconv.Atoi(match[1])
		// Remove the old suffix and add the new incremented one
		newBase := reDup.ReplaceAllString(baseName, fmt.Sprintf("-%d", num+1))
		return newBase + ext
	}

	// If no suffix exists, start with -1
	return baseName + "-1" + ext
}

func CopyFile(src, dst string) error {
	// Open the source file for reading
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close() //nolint:errcheck

	destinationFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close() //nolint:errcheck

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	err = destinationFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}
