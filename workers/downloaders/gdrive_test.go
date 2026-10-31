package downloaders

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"enc/internal/bot/types"
	"enc/internal/clients/gdrive"
	"enc/internal/logger"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "downloaders-test")
	if err != nil {
		panic(err)
	}
	if err := logger.Init(filepath.Join(dir, "log.txt"), false); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// fakeDrive serves public Drive pages: a single file, and a folder with a
// subfolder and an entry titled to escape the folder.
func fakeDrive(t *testing.T) {
	t.Helper()
	content := map[string]string{
		"singlefile001": "single file body",
		"episode000001": "episode one",
		"episode000002": "episode two",
		"escapefile001": "should stay inside",
	}
	names := map[string]string{
		"singlefile001": "Show - 01.mkv",
		"episode000001": "Ep 01.mkv",
		"episode000002": "Ep 02.mkv",
		"escapefile001": "evil.mkv",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		body, ok := content[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+names[id]+`"`)
		if r.Header.Get("Range") == "bytes=0-0" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(body[:1]))
			return
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/embeddedfolderview", func(w http.ResponseWriter, r *http.Request) {
		entry := func(id, href, title string) string {
			return `<div class="flip-entry" id="entry-` + id + `"><a href="` + href + `"><div class="flip-entry-title">` + title + `</div></a></div>`
		}
		switch r.URL.Query().Get("id") {
		case "seasonfolder1":
			_, _ = w.Write([]byte(`<title>Season 1</title><div class="flip-entries">` +
				entry("episode000001", "https://drive.google.com/file/d/episode000001/view", "Ep 01.mkv") +
				entry("subfolder0001", "https://drive.google.com/drive/folders/subfolder0001", "..") +
				`</div>`))
		case "subfolder0001":
			_, _ = w.Write([]byte(`<title>..</title><div class="flip-entries">` +
				entry("episode000002", "https://drive.google.com/file/d/episode000002/view", "Ep 02.mkv") +
				entry("escapefile001", "https://drive.google.com/file/d/escapefile001/view", "../../evil.mkv") +
				`</div>`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := gdrive.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	old := gdriveShared
	gdriveOnce.Do(func() {}) // keep the real client from being built
	gdriveShared = c.WithBaseURL(srv.URL)
	t.Cleanup(func() { gdriveShared = old })
}

func newTestGDriveDl(t *testing.T, uri, selected string) (*GDriveDownload, string) {
	t.Helper()
	dir := t.TempDir()
	return NewGDriveDl(uri, dir, selected, nil, &types.TransferInfo{}), dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGDriveSingleFile(t *testing.T) {
	fakeDrive(t)
	dl, dir := newTestGDriveDl(t, "https://drive.google.com/file/d/singlefile001/view?usp=sharing", "")
	if err := dl.Start(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Show - 01.mkv")
	if dl.Path != want || dl.GetFileName() != "Show - 01.mkv" {
		t.Errorf("Path = %q, want %q", dl.Path, want)
	}
	if got := readFile(t, want); got != "single file body" {
		t.Errorf("content = %q", got)
	}
	dl.Clean()
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Error("Clean left the file")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("Clean removed the download dir itself")
	}
}

func TestGDriveFolder(t *testing.T) {
	fakeDrive(t)
	dl, dir := newTestGDriveDl(t, "https://drive.google.com/drive/folders/seasonfolder1", "")
	if err := dl.Start(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "Season 1")
	if dl.Path != root {
		t.Errorf("Path = %q, want the folder %q", dl.Path, root)
	}
	// ".." titles become "_", so nothing lands outside the folder.
	for path, body := range map[string]string{
		"Ep 01.mkv":      "episode one",
		"_/Ep 02.mkv":    "episode two",
		"_/_.._evil.mkv": "should stay inside",
	} {
		if got := readFile(t, filepath.Join(root, filepath.FromSlash(path))); got != body {
			t.Errorf("%s = %q, want %q", path, got, body)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.mkv")); !os.IsNotExist(err) {
		t.Error("a file escaped the folder")
	}
	dl.Clean()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("Clean left %v", entries)
	}
}

func TestGDriveSelectedFile(t *testing.T) {
	fakeDrive(t)
	// -s is 1-based, in /list order: Ep 01, then the subfolder's files.
	dl, dir := newTestGDriveDl(t, "https://drive.google.com/drive/folders/seasonfolder1", "2")
	if err := dl.Start(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Season 1", "_", "Ep 02.mkv")
	if dl.Path != want || readFile(t, want) != "episode two" {
		t.Errorf("Path = %q, want %q", dl.Path, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "Season 1", "Ep 01.mkv")); !os.IsNotExist(err) {
		t.Error("an unselected file was downloaded")
	}

	for _, bad := range []string{"0", "4", "x"} {
		dl, _ := newTestGDriveDl(t, "https://drive.google.com/drive/folders/seasonfolder1", bad)
		if err := dl.Start(); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("-s %s: err = %v", bad, err)
		}
	}
}

func TestGetGDriveDlInfo(t *testing.T) {
	fakeDrive(t)
	info := GetGDriveDlInfo("https://drive.google.com/drive/folders/seasonfolder1")
	if info.Error != nil {
		t.Fatal(info.Error)
	}
	want := []string{"Season 1/Ep 01.mkv", "Season 1/_/Ep 02.mkv", "Season 1/_/_.._evil.mkv"}
	if info.Name != "Season 1" || info.Count != 3 || strings.Join(info.Files, "|") != strings.Join(want, "|") {
		t.Errorf("folder info = %+v", info)
	}
	info = GetGDriveDlInfo("https://drive.google.com/file/d/singlefile001/view")
	if info.Error != nil || info.Count != 1 || info.Name != "Show - 01.mkv" {
		t.Errorf("file info = %+v", info)
	}
	if info := GetGDriveDlInfo("https://drive.google.com/file/d/missingfile01/view"); info.Error == nil {
		t.Error("missing file gave no error")
	}
}
