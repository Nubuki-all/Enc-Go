package downloaders

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"enc/config"
	"enc/internal/bot/types"
	"enc/internal/clients/gdrive"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

var (
	gdriveOnce   sync.Once
	gdriveShared *gdrive.Client
)

// gdriveClient returns the shared Drive client, with the service account
// from GDRIVE_SA_JSON (a key file's path, or its JSON) when it's set and
// valid, and public links only otherwise.
func gdriveClient() *gdrive.Client {
	gdriveOnce.Do(func() {
		var key []byte
		if sa := strings.TrimSpace(config.Conf.GDriveSA); sa != "" {
			if strings.HasPrefix(sa, "{") {
				key = []byte(sa)
			} else if b, err := os.ReadFile(sa); err == nil {
				key = b
			} else {
				logger.Errorf("GDRIVE_SA_JSON: %v; using public Drive links only", err)
			}
		}
		c, err := gdrive.New(key)
		if err != nil {
			logger.Errorf("%v; using public Drive links only", err)
			c, _ = gdrive.New(nil)
		}
		gdriveShared = c
	})
	return gdriveShared
}

// gdriveFiles lists what a Drive link downloads: one file, or a folder's
// files (paths inside the folder) and the folder's name.
func gdriveFiles(ctx context.Context, uri string) (link gdrive.Link, name string, entries []gdrive.Entry, err error) {
	client := gdriveClient()
	link, err = client.Identify(ctx, uri)
	if err != nil {
		return link, "", nil, err
	}
	if link.Kind == gdrive.File {
		r, err := client.Resolve(ctx, link.ID, "")
		if err != nil {
			return link, "", nil, err
		}
		name := utils.SafeFileName(r.Name)
		if name == "" {
			name = link.ID
		}
		return link, name, []gdrive.Entry{{ID: link.ID, Path: name, Size: r.Size}}, nil
	}
	listing, err := client.List(ctx, link.ID)
	if err != nil {
		return link, "", nil, err
	}
	if len(listing.Entries) == 0 {
		return link, "", nil, errors.New("this Drive folder has no files to download")
	}
	name = utils.SafeFileName(listing.Name)
	if name == "" {
		name = "Google Drive"
	}
	return link, name, listing.Entries, nil
}

// GetGDriveDlInfo lists a Drive link's files for /list, -s and batches.
// Folder files are listed under the folder's name, like a torrent's.
func GetGDriveDlInfo(uri string) *utils.DLInfo {
	dlInfo := &utils.DLInfo{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	link, name, entries, err := gdriveFiles(ctx, uri)
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}
	dlInfo.Name = name
	for _, e := range entries {
		if link.Kind == gdrive.Folder {
			dlInfo.Files = append(dlInfo.Files, name+"/"+e.Path)
		} else {
			dlInfo.Files = append(dlInfo.Files, e.Path)
		}
	}
	dlInfo.Count = len(dlInfo.Files)
	if dlInfo.Count == 1 {
		dlInfo.Name = filepath.Base(dlInfo.Files[0])
	}
	return dlInfo
}

type GDriveDownload struct {
	*types.TransferInfo
	msg      *tele.Message
	ufstr    string
	fstr     string
	dir      string
	basePath string
	selected string
	uri      string
	started  bool
}

func NewGDriveDl(uri, dir, selected string, msg *tele.Message, tInfo *types.TransferInfo) *GDriveDownload {
	conf := config.Conf
	return &GDriveDownload{
		TransferInfo: tInfo,
		msg:          msg,
		dir:          dir,
		selected:     selected,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
		uri:          uri,
	}
}

func (dl *GDriveDownload) GetFileName() string {
	return filepath.Base(dl.Path)
}

func (dl *GDriveDownload) AttachMsg(msg *tele.Message) {
	dl.msg = msg
}

// localPath turns a Drive path into one under root, part by part, so a
// title like ".." can't leave it.
func localPath(root, drivePath string) string {
	path := root
	for _, part := range strings.Split(drivePath, "/") {
		if part = utils.SafeFileName(part); part != "" {
			path = filepath.Join(path, part)
		}
	}
	return path
}

func (dl *GDriveDownload) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	link, name, entries, err := gdriveFiles(ctx, dl.uri)
	if err != nil {
		return err
	}
	dl.basePath = filepath.Join(dl.dir, name)
	dl.Path = dl.basePath
	if dl.selected != "" && len(entries) > 1 {
		s, err := strconv.Atoi(dl.selected)
		if err != nil || s < 1 || s > len(entries) {
			return fmt.Errorf("gdrive: file_id %s not found", dl.selected)
		}
		entries = entries[s-1 : s]
	}
	if link.Kind == gdrive.Folder && len(entries) == 1 && dl.selected != "" {
		dl.Path = localPath(dl.basePath, entries[0].Path)
	}
	if !utils.IsInsideDir(dl.dir, dl.basePath) {
		return fmt.Errorf("gdrive: refusing to download outside %s", dl.dir)
	}
	dl.started = true
	for i, e := range entries {
		path := dl.basePath
		if link.Kind == gdrive.Folder {
			path = localPath(dl.basePath, e.Path)
		}
		if err := dl.downloadFile(ctx, e, path, i+1, len(entries)); err != nil {
			if dl.IsCancelled.Load() {
				return nil
			}
			return fmt.Errorf("%s: %w", e.Path, err)
		}
		if dl.IsCancelled.Load() {
			return nil
		}
	}
	return nil
}

// downloadFile fetches one file to path, with a fresh link (links to large
// public files don't last).
func (dl *GDriveDownload) downloadFile(ctx context.Context, e gdrive.Entry, path string, n, total int) error {
	r, err := gdriveClient().Resolve(ctx, e.ID, e.Native)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return err
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("google Drive: HTTP %d", resp.StatusCode)
	}
	size := r.Size
	if size < 0 {
		size = resp.ContentLength
	}

	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	label := filepath.Base(path)
	if total > 1 {
		label = fmt.Sprintf("%s (%d/%d)", label, n, total)
	}
	start := time.Now()
	var written int64
	interval := config.Rconf.ProgresUpdateInterval * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	buf := make([]byte, 256<<10)
	for {
		if dl.IsCancelled.Load() {
			err = ErrDownloadCanceled
			break
		}
		var nr int
		nr, err = resp.Body.Read(buf)
		if nr > 0 {
			if _, werr := out.Write(buf[:nr]); werr != nil {
				err = werr
				break
			}
			written += int64(nr)
		}
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			break
		}
		select {
		case <-ticker.C:
			dl.ProgressCallback(start, label, size, written)
		default:
		}
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && size > 0 && written != size {
		err = fmt.Errorf("got %d of %d bytes", written, size)
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

func (dl *GDriveDownload) ProgressCallback(start time.Time, name string, total, current int64) {
	dl.Mutex.Lock()
	defer dl.Mutex.Unlock()
	if dl.PauseProgress.Load() || dl.msg == nil {
		return
	}
	diff := time.Since(start).Round(time.Second).Seconds()
	var speed int64
	if diff > 0 {
		speed = current / int64(diff)
	}
	var percentage float64
	timeToCompletion := "0 s"
	remainingSize := total - current
	if total > 0 {
		percentage = float64(current * 100 / total)
		if speed > 0 && remainingSize > 0 {
			timeToCompletion = utils.TimeFormatter(float64(remainingSize) / float64(speed))
		}
	} else {
		remainingSize = 0
	}
	elapsedTime := utils.TimeFormatter(diff)
	if elapsedTime == "" {
		elapsedTime = "0 s"
	}
	filled := int(math.Floor(percentage / 10.0))
	finStr := dl.fstr
	if finStr == "" {
		finStr = random.EnHearts()
	}
	bar := strings.Repeat(finStr, filled) + strings.Repeat(dl.ufstr, 10-filled)
	totalStr := "?"
	if total > 0 {
		totalStr = utils.VHBS(total)
	}
	txt := fmt.Sprintf(
		"**Downloading:**\n`%s`\n**via:** Google Drive\n```\n%s```\n**Progress:** `%.2f%%`\n`%s of %s`\n**Speed:** `%s/s`\n**Remains:** `%s`\n**ETA:** `%s`\n**Elapsed:** `%s`\n",
		name, bar, percentage,
		utils.VHBS(current), totalStr,
		utils.VHBS(speed),
		utils.VHBS(remainingSize),
		timeToCompletion,
		elapsedTime,
	)
	_, _ = dl.msg.EditText(txt)
}

func (dl *GDriveDownload) Clean() {
	if !dl.started || dl.basePath == "" || !utils.IsInsideDir(dl.dir, dl.basePath) {
		logger.WarnW("GDrive Dl: Calling Clean on a download that exited early")
		return
	}
	_ = os.RemoveAll(dl.basePath)
}
