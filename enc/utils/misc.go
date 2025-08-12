package utils

import (
	//"bufio"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
)

// Regex patterns
var (
	MagnetRegex = regexp.MustCompile(`magnet:\?xt=urn:[a-z0-9]+:[a-zA-Z0-9]{32}`)
	URLRegex    = regexp.MustCompile(`^(https?://|ftp://)?(www\.)?[^/\s]+\.[^/\s:]+(:\d+)?(/[^?\s]*[\s\S]*)?(\?[^#\s]*[\s\S]*)?(#.*)?$`)
)

// IsMagnet checks if the given link is a magnet URL
func IsMagnet(link string) bool {
	return MagnetRegex.MatchString(link)
}

// IsURL checks if the given string is a URL
func IsURL(u string) bool {
	return URLRegex.MatchString(u)
}

// ReplaceProxy reads replace_proxy.txt and applies proxy rules
func ReplaceProxy(url string) string {
	file := "replace_proxy.txt"
	if url == "" || !FileExists(file) {
		return url
	}
	lines, err := readLines(file)
	if err != nil {
		return url
	}
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		search, proxy := parts[0], parts[1]
		if strings.HasPrefix(url, search) {
			if strings.HasSuffix(proxy, "=") {
				return proxy + url
			}
			return strings.Replace(url, search, proxy, 1)
		}
	}
	return url
}

// CheckCmds returns true if command ends exactly with any of the matches plus suffix
func CheckCmds(command string, matches ...string) bool {
	for _, m := range matches {
		full := m + config.Conf.CmdSuffix
		if strings.HasSuffix(command, full) {
			return true
		}
	}
	return false
}

// Gfn returns the fully-qualified name of a function (package path + name)
func Gfn(fn interface{}) string {
	// Not directly possible in Go; return placeholder
	return fmt.Sprintf("%T", fn)
}

// StringEscape unescapes escaped characters in a string
func StringEscape(s string) string {
	// Go strings handle escapes; as placeholder simply return original
	return s
}

// StringEscape unescapes escaped characters in a string
func stringEscape(s string) string {
	if s == "" {
		return s
	}
	// Wrap in quotes so strconv.Unquote can process escape sequences
	quoted := "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	unq, err := strconv.Unquote(quoted)
	if err != nil {
		return s
	}
	return unq
}

// ListToStr joins a slice of items into a string, optionally numbered
func ListToStr(lst []string, sep string, start int, numbered bool) string {
	if sep == "" {
		sep = " "
	}
	var sb strings.Builder
	if numbered {
		for i, item := range lst {
			sb.WriteString(fmt.Sprintf("%d. `%s`%s", start+i, item, sep))
		}
	} else {
		for _, item := range lst {
			sb.WriteString(item + sep)
		}
	}
	result := sb.String()
	return strings.TrimSuffix(result, sep)
}

// TxtToStr reads a file and returns its contents as a string
func TxtToStr(path string) (string, error) {
	if !FileExists(path) {
		return "", nil
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// File extension checks
func IsAudioFile(name string) bool {
	exts := []string{".aac", ".mp3", ".m4a", ".flac", ".opus", ".wav"}
	return hasExt(name, exts)
}

func IsSubtitleFile(name string) bool {
	exts := []string{".ass", ".srt", ".txt", ".pgs"}
	return hasExt(name, exts)
}

func IsVideoFile(name string) bool {
	exts := []string{
		".3g2", ".3gp", ".3gp2", ".3gpp", ".avc", ".avd", ".avi",
		".evo", ".fli", ".flv", ".flx", ".m4u", ".m4v", ".mkv",
		".mov", ".movie", ".mp21", ".mp2v", ".mp4", ".mp4v", ".mpeg",
		".mpg", ".mpg2", ".xvid",
	}
	return hasExt(name, exts)
}

func IsSupportedFile(name string) bool {
	return IsAudioFile(name) || IsSubtitleFile(name) || IsVideoFile(name)
}

// GetReadableFileSize formats bytes as human-readable units
func GetReadableFileSize(size int64) string {
	if size <= 0 {
		return "0B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	var i int
	fsize := float64(size)
	for fsize >= 1024 && i < len(units)-1 {
		fsize /= 1024
		i++
	}
	return fmt.Sprintf("%.2f%s", fsize, units[i])
}

// helpers
//func fileExists(path string) bool {
//	info, err := os.Stat(path)
//	return err == nil && !info.IsDir()
//}

func readLines(path string) ([]string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), nil
}

func hasExt(name string, exts []string) bool {
	lower := strings.ToLower(filepath.Ext(name))
	for _, e := range exts {
		if lower == e {
			return true
		}
	}
	return false
}

// GetHTML fetches the HTML content of the given link.
func GetHTML(link string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Get(ReplaceProxy(link))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// SplitText splits text into chunks of up to maxLen characters, preserving delimiters.
func SplitText(text, sep string, pre bool) []string {
	const maxLen = 4000
	var (
		current strings.Builder
		chunks  []string
	)
	for _, part := range strings.Split(text, sep) {
		line := part + sep
		if pre {
			line = sep + part
		}
		if current.Len()+len(line) <= maxLen {
			current.WriteString(line)
		} else {
			chunks = append(chunks, current.String())
			current.Reset()
			current.WriteString(line)
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

// CodeStorage holds arbitrary data entries indexed by integer keys.
type CodeEntry struct {
	Data    string
	InFile  string
	OutFile string
	User    interface{}
	Stime   interface{}
}

var (
	Storage = make(map[int]CodeEntry)
	mutex   sync.RWMutex
)

// Code stores or updates an entry. Without index, it appends and returns its key.
func Code(data, infile, outfile string, user, stime interface{}, index ...int) (string, error) {
	mutex.Lock()
	defer mutex.Unlock()
	if len(index) == 0 {
		key := len(Storage)
		Storage[key] = CodeEntry{Data: data}
		return strconv.Itoa(key), nil
	}
	idx := index[0]
	if _, ok := Storage[idx]; !ok {
		return "", errors.New("invalid index")
	}
	Storage[idx] = CodeEntry{Data: data, InFile: infile, OutFile: outfile, User: user, Stime: stime}
	return "", nil
}

// Decode retrieves (and optionally removes) an entry by key.
func Decode(key interface{}, pop bool) (CodeEntry, bool) {
	mutex.Lock()
	defer mutex.Unlock()
	var idx int
	switch v := key.(type) {
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return CodeEntry{}, false
		}
		idx = i
	case int:
		idx = v
	default:
		return CodeEntry{}, false
	}
	entry, ok := Storage[idx]
	if !ok {
		return CodeEntry{}, false
	}
	if pop {
		delete(Storage, idx)
	}
	return entry, true
}

// ValueCheck returns a placeholder if the value is empty.
func ValueCheck(val string) string {
	if val == "" {
		return "-"
	}
	return val
}

// stdr formats seconds into HH:MM:SS.
func stdr(seconds int) string {
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// TimeFormatter humanizes seconds into d, h, m, s.
func TimeFormatter(sec float64) string {
	d := time.Duration(int(sec)) * time.Second
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	parts = append(parts, fmt.Sprintf("%ds", s))
	return strings.Join(parts, ", ")
}

// TimeFormatterMS humanizes milliseconds into d, h, m, s. This mirrors the Python TimeFormatter.
func TimeFormatterMS(ms int) string {
	return TimeFormatter(float64(ms) / 1000)
}

// TS formats milliseconds into d, h, m, s, ms.
func TS(ms int) string {
	d := time.Duration(ms) * time.Millisecond
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	millis := ms % 1000
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	parts = append(parts, fmt.Sprintf("%dms", millis))
	return strings.Join(parts, ", ")
}

// HBS returns a human-readable byte size string.
func HBS(size int64) string {
	if size <= 0 {
		return ""
	}
	units := []string{"B", "K", "M", "G", "T", "P"}
	i := 0
	sz := float64(size)
	for sz >= 1024 && i < len(units)-1 {
		sz /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %sB", sz, units[i])
}

// CRC32 computes the CRC-32 checksum of a file.
func CRC32(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	chk := crc32.NewIEEE()
	if _, err := io.Copy(chk, file); err != nil {
		return "", err
	}
	return strings.ToUpper(fmt.Sprintf("%X", chk.Sum32())), nil
}

// GetCodecs returns combined codec strings for multiple files.
func GetCodecs(slices []string) string {
	var out []string
	for _, s := range slices {
		c := GetCodec(s)
		out = append(out, c)
	}
	return strings.Join(out, ", ")
}

// GetCodec extracts codec tags from an ffmpeg output file.
func GetCodec(str string) string {
	tags := map[string]string{
		"360":       "360p",
		"480":       "480p",
		"720":       "720p",
		"1080":      "1080p",
		"x264":      "AVC",
		"x265":      "HEVC",
		"libsvtav1": "AV1",
		"svt_av1":   "AV1",
	}
	var out []string
	for k, v := range tags {
		if strings.Contains(str, k) {
			out = append(out, "["+v+"]")
		}
	}
	return strings.Join(out, " ")
}

// AutoRename applies renaming rules based on Renamefilters.
func AutoRename(parsedName, originalName string, caption, banner bool) (bool, string) {
	var filter *types.RenameFilterData
	for _, rf := range bot.RenameFilters {
		if rf.CaseSensitive == true {
			if strings.ToLower(rf.Match) != strings.ToLower(originalName) {
				continue
			}
		} else if rf.Match != originalName {
			continue
		} else {
			continue
		}
		filter = rf
		break
	}
	if filter == nil {
		return false, originalName
	}
	defaultFunc := func() string {
		if filter.ReplaceName == "00" {
			return originalName
		} else {
			return filter.ReplaceName
		}
	}
	captionFunc := func() string {
		capOpt := filter.ReplaceCaption
		var outCap string
		if capOpt == "0" {
			outCap = parsedName
		} else if capOpt == "1" || capOpt == "" {
			outCap = defaultFunc()
		} else if capOpt == "00" {
			outCap = originalName
		} else {
			outCap = capOpt
		}
		return outCap
	}
	bannerFunc := func() string {
		bannerOpt := filter.ReplaceBanner
		var outStr string
		if bannerOpt == "1" {
			outStr = captionFunc()
		} else if bannerOpt == "0" {
			outStr = defaultFunc()
		} else if bannerOpt == "" || bannerOpt == "00" {
			outStr = originalName
		} else {
			outStr = bannerOpt
		}
		return outStr
	}
	if caption {
		return filter.DisableAnilist, captionFunc()
	} else if banner {
		return filter.DisableAnilist, bannerFunc()
	} else {
		return filter.DisableAnilist, defaultFunc()
	}
}

// TextFilter reads three filter files and returns their contents.
func TextFilter() (nameFilter, releaseFilter, captionFilter string, err error) {
	files := map[string]*string{
		"filters/Namefilter.txt":      &nameFilter,
		"filters/Releasefilter.txt":   &releaseFilter,
		"filters/Release_caption.txt": &captionFilter,
	}
	for path, ptr := range files {
		if _, err := os.Stat(path); err == nil {
			data, e := ioutil.ReadFile(path)
			if e != nil {
				return "", "", "", e
			}
			*ptr = strings.TrimSpace(string(data))
		} else if os.IsNotExist(err) {
			*ptr = ""
		} else {
			return "", "", "", err
		}
	}
	return nameFilter, releaseFilter, captionFilter, nil
}
