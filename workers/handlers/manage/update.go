package manage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

// maxBinarySize guards against downloading something that can't be the bot.
const maxBinarySize = 512 << 20

var errUpToDate = errors.New("already up to date")

// Variables so tests can use a local server and a fixed platform.
var (
	githubAPI    = "https://api.github.com"
	goos, goarch = runtime.GOOS, runtime.GOARCH
)

// Update replaces the running binary with the matching asset of the upstream
// repo's "latest" release, then restarts.
func Update(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(
			"Downloads the latest release from UPSTREAM_REPO, replaces the bot's binary and restarts.\n"+
				"Does nothing if the running binary is already the latest one.", false))
		return nil
	}
	rMsg, err := msg.ReplyText("`Checking for updates…`")
	if err != nil {
		return nil
	}

	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		_, _ = rMsg.EditText("**Update failed:** " + err.Error())
		return nil
	}

	dctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	asset, err := downloadLatest(dctx, exe)
	switch {
	case errors.Is(err, errUpToDate):
		_, _ = rMsg.EditText("`Already running the latest release.`")
		return nil
	case err != nil:
		_, _ = rMsg.EditText("**Update failed:** " + err.Error())
		return nil
	}

	rMsg, _ = rMsg.EditText("`Updated to " + asset + ", restarting…`")
	downloaders.CleanAllQbit(ctx)
	db.SaveAll(ctx)
	if bot.Aria2Client != nil {
		bot.Aria2Client.Close() //nolint:errcheck
	}
	if err := utils.Restart(msg.ChatID, rMsg.ID, config.Rconf.Detached); err != nil {
		_, _ = rMsg.EditText("**Updated, but the restart failed:** " + err.Error())
	}
	return nil
}

// releaseAssetName is the name the build workflow gives the binary for a platform.
func releaseAssetName(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && (goarch == "amd64" || goarch == "arm64"):
		return "enc-linux-" + goarch + "-musl", nil
	case goos == "linux" && goarch == "arm":
		return "enc-linux-armv7-musl", nil
	case goos == "windows" && (goarch == "amd64" || goarch == "arm64"):
		return "enc-windows-" + goarch + ".exe", nil
	}
	return "", fmt.Errorf("no release builds for %s/%s", goos, goarch)
}

// parseRepo accepts "owner/repo" or a github.com URL and returns "owner/repo".
func parseRepo(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("UPSTREAM_REPO %q is not owner/repo", s)
	}
	return s, nil
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"` // API url; serves the file with Accept: application/octet-stream
	Size int64  `json:"size"`
}

func githubRequest(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token := config.Conf.UpstreamToken; token != "" {
		// Go drops this header when following the redirect to the file host.
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound && config.Conf.UpstreamToken == "" {
			return nil, fmt.Errorf("%s: not found (a private UPSTREAM_REPO needs UPSTREAM_TOKEN)", url)
		}
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return resp, nil
}

// downloadLatest fetches the release asset for this platform and, unless it
// matches exe, puts it in exe's place. It returns the asset name.
func downloadLatest(ctx context.Context, exe string) (string, error) {
	repo, err := parseRepo(config.Conf.UpstreamRepo)
	if err != nil {
		return "", err
	}
	name, err := releaseAssetName(goos, goarch)
	if err != nil {
		return "", err
	}

	resp, err := githubRequest(ctx, githubAPI+"/repos/"+repo+"/releases/tags/latest", "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var release struct {
		Assets []releaseAsset `json:"assets"`
	}
	err = json.NewDecoder(resp.Body).Decode(&release)
	_ = resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("reading release: %w", err)
	}
	var asset *releaseAsset
	for i := range release.Assets {
		if release.Assets[i].Name == name {
			asset = &release.Assets[i]
		}
	}
	if asset == nil {
		return "", fmt.Errorf("the latest release has no %s", name)
	}
	if asset.Size > maxBinarySize {
		return "", fmt.Errorf("%s is %d bytes, too large", name, asset.Size)
	}

	resp, err = githubRequest(ctx, asset.URL, "application/octet-stream")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// Write next to exe so the final rename stays on one filesystem.
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".enc-update-*")
	if err != nil {
		return "", err
	}
	done := false
	defer func() {
		if !done {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxBinarySize+1))
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", name, err)
	}
	if n > maxBinarySize {
		return "", fmt.Errorf("%s is larger than %d bytes", name, maxBinarySize)
	}
	if asset.Size > 0 && n != asset.Size {
		return "", fmt.Errorf("%s: got %d of %d bytes", name, n, asset.Size)
	}
	if err := checkExecutable(tmp, goos); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}

	current, err := fileSHA256(exe)
	if err != nil {
		return "", err
	}
	if bytes.Equal(current, h.Sum(nil)) {
		return "", errUpToDate
	}

	mode := os.FileMode(0o755)
	if fi, err := os.Stat(exe); err == nil {
		mode = fi.Mode().Perm() | 0o100
	}
	if err := tmp.Chmod(mode); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	// The running binary can't be written to, but it can be renamed: on
	// Linux the new file is renamed over it; Windows won't replace a running
	// .exe, so the old one is moved aside first. The restart then runs the
	// new file.
	if goos == "windows" {
		old := exe + ".old"
		_ = os.Remove(old) // left by the previous update
		if err := os.Rename(exe, old); err != nil {
			return "", err
		}
		if err := os.Rename(tmp.Name(), exe); err != nil {
			_ = os.Rename(old, exe)
			return "", err
		}
	} else if err := os.Rename(tmp.Name(), exe); err != nil {
		return "", err
	}
	done = true
	return name, nil
}

// checkExecutable rejects downloads that aren't an executable for goos
// (an ELF binary on Linux, a PE "MZ" binary on Windows).
func checkExecutable(f *os.File, goos string) error {
	magic := []byte("\x7fELF")
	if goos == "windows" {
		magic = []byte("MZ")
	}
	head := make([]byte, len(magic))
	if _, err := f.ReadAt(head, 0); err != nil {
		return fmt.Errorf("reading header: %w", err)
	}
	if !bytes.Equal(head, magic) {
		return fmt.Errorf("not a %s executable", goos)
	}
	return nil
}

func fileSHA256(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
