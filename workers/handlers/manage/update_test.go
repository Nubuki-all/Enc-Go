package manage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"enc/config"
)

func TestReleaseAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
		wantErr            bool
	}{
		{"linux", "amd64", "enc-linux-amd64-musl", false},
		{"linux", "arm64", "enc-linux-arm64-musl", false},
		{"linux", "arm", "enc-linux-armv7-musl", false},
		{"linux", "386", "", true},
		{"windows", "amd64", "", true},
	}
	for _, tt := range tests {
		got, err := releaseAssetName(tt.goos, tt.goarch)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("releaseAssetName(%s, %s) = %q, %v", tt.goos, tt.goarch, got, err)
		}
	}
}

func TestParseRepo(t *testing.T) {
	for in, want := range map[string]string{
		"Nubuki-all/Enc-Go":                        "Nubuki-all/Enc-Go",
		"https://github.com/Nubuki-all/Enc-Go":     "Nubuki-all/Enc-Go",
		"https://github.com/Nubuki-all/Enc-Go.git": "Nubuki-all/Enc-Go",
		" github.com/a/b/ ":                        "a/b",
	} {
		if got, err := parseRepo(in); err != nil || got != want {
			t.Errorf("parseRepo(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "repo", "a/b/c", "/b"} {
		if _, err := parseRepo(in); err == nil {
			t.Errorf("parseRepo(%q) succeeded", in)
		}
	}
}

// fakeRelease serves a "latest" release with one asset for this platform.
func fakeRelease(t *testing.T, body []byte, size int64, token string) {
	t.Helper()
	oldOS, oldArch := goos, goarch
	t.Cleanup(func() { goos, goarch = oldOS, oldArch })
	goos, goarch = "linux", "amd64"
	name, err := releaseAssetName(goos, goarch)
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/repos/o/r/releases/tags/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []map[string]any{
				{"name": "other", "url": srv.URL + "/nope", "size": 1},
				{"name": name, "url": srv.URL + "/asset", "size": size},
			}})
		case "/asset":
			if r.Header.Get("Accept") != "application/octet-stream" {
				http.Error(w, "wrong accept", http.StatusBadRequest)
				return
			}
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	oldAPI, oldRepo, oldToken := githubAPI, config.Conf.UpstreamRepo, config.Conf.UpstreamToken
	t.Cleanup(func() { githubAPI, config.Conf.UpstreamRepo, config.Conf.UpstreamToken = oldAPI, oldRepo, oldToken })
	githubAPI = srv.URL
	config.Conf.UpstreamRepo = "o/r"
	config.Conf.UpstreamToken = token
}

func writeExe(t *testing.T, content string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "enc")
	if err := os.WriteFile(exe, []byte(content), 0o750); err != nil {
		t.Fatal(err)
	}
	return exe
}

func leftovers(t *testing.T, exe string) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".enc-update-*"))
	return m
}

func TestDownloadLatestReplaces(t *testing.T) {
	newBin := "\x7fELF new build"
	fakeRelease(t, []byte(newBin), int64(len(newBin)), "secret")
	exe := writeExe(t, "\x7fELF old build")

	if _, err := downloadLatest(context.Background(), exe); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != newBin {
		t.Errorf("exe = %q, want the new build", got)
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("new exe is not executable: %v", fi.Mode())
	}
	if l := leftovers(t, exe); len(l) != 0 {
		t.Errorf("temp files left: %v", l)
	}
}

func TestDownloadLatestUpToDate(t *testing.T) {
	bin := "\x7fELF same build"
	fakeRelease(t, []byte(bin), int64(len(bin)), "")
	exe := writeExe(t, bin)

	if _, err := downloadLatest(context.Background(), exe); !errors.Is(err, errUpToDate) {
		t.Fatalf("err = %v, want errUpToDate", err)
	}
	if l := leftovers(t, exe); len(l) != 0 {
		t.Errorf("temp files left: %v", l)
	}
}

func TestDownloadLatestRejectsBadDownloads(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		size    int64
		wantErr string
	}{
		{"not an executable", "<html>login</html>", 18, "not a Linux executable"},
		{"truncated", "\x7fELF part", 100, "got 9 of 100 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRelease(t, []byte(tt.body), tt.size, "")
			old := "\x7fELF old build"
			exe := writeExe(t, old)

			_, err := downloadLatest(context.Background(), exe)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if got, _ := os.ReadFile(exe); string(got) != old {
				t.Errorf("exe changed to %q after a failed update", got)
			}
			if l := leftovers(t, exe); len(l) != 0 {
				t.Errorf("temp files left: %v", l)
			}
		})
	}
}

func TestDownloadLatestPrivateRepoHint(t *testing.T) {
	fakeRelease(t, nil, 0, "needed")
	config.Conf.UpstreamToken = ""
	_, err := downloadLatest(context.Background(), writeExe(t, "\x7fELF"))
	if err == nil || !strings.Contains(err.Error(), "UPSTREAM_TOKEN") {
		t.Fatalf("err = %v, want a hint about UPSTREAM_TOKEN", err)
	}
}
