package downloaders

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	qbit "github.com/autobrr/go-qbittorrent"
	"github.com/google/uuid"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/lrstanley/go-ytdlp"
	"github.com/siku2/arigo"

	"enc/config"
	"enc/internal/bot"
	// "enc/internal/logger"
	"enc/utils"
)

var ErrDownloadCanceled = errors.New("download canceled by user")

// ProgressFunc is called on progress events. start is the download start time,
// done is bytes downloaded so far, total is total size (0 if unknown).
type ProgressFunc func(start time.Time, done, total int64, w *progressWriter, wa *progressWriterAt)

// ================= progress writers ======================

// progressWriter is an io.Writer that counts bytes and calls cb(start, done, total).
type progressWriter struct {
	w       io.Writer
	start   time.Time
	Cancel  bool
	total   int64
	written int64
	cb      ProgressFunc
}

func NewProgressWriter(w io.Writer, total int64, cb ProgressFunc) *progressWriter {
	return &progressWriter{
		w:     w,
		start: time.Now(),
		total: total,
		cb:    cb,
	}
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if n > 0 {
		new := atomic.AddInt64(&p.written, int64(n))
		if p.cb != nil {
			p.cb(p.start, new, p.total, p, nil)
		}
		if p.Cancel {
			return n, ErrDownloadCanceled
		}
	}
	return n, err
}

// progressWriterAt is an io.WriterAt that delegates to an *os.File.WriteAt
// and reports progress via callback (safe for parallel downloads).
type progressWriterAt struct {
	f       *os.File
	start   time.Time
	Cancel  bool
	total   int64
	written int64
	cb      ProgressFunc
}

func NewProgressWriterAt(f *os.File, total int64, cb ProgressFunc) *progressWriterAt {
	return &progressWriterAt{
		f:     f,
		start: time.Now(),
		total: total,
		cb:    cb,
	}
}

func (p *progressWriterAt) WriteAt(b []byte, off int64) (int, error) {
	n, err := p.f.WriteAt(b, off)
	if n > 0 {
		done := atomic.AddInt64(&p.written, int64(n))
		if p.cb != nil {
			p.cb(p.start, done, p.total, nil, p)
		}
		if p.Cancel {
			return n, ErrDownloadCanceled
		}
	}
	return n, err
}

// ================= download helpers (examples) ======================

// DownloadStream downloads using sequential Stream mode.
// `d` is a downloader.Downloader, `client` must satisfy downloader.Client
// `loc` is the tg.InputFileLocationClass you supply (can be built via BuildInputFileLocation)
func DownloadStream(ctx context.Context, d *downloader.Downloader, client downloader.Client, loc tg.InputFileLocationClass, size int64, outPath string, cb ProgressFunc) error {
	if d == nil || client == nil || loc == nil {
		return errors.New("missing downloader/client/loc")
	}
	// Build destination file
	dirPath := filepath.Dir(outPath)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}
	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close() //nolint:errcheck

	pw := NewProgressWriter(outFile, size, cb)

	builder := d.Download(client, loc)
	// Stream writes sequentially into pw
	_, err = builder.Stream(ctx, pw)
	return err
}

// DownloadParallel uses Parallel download (concurrent parts).
// threads: number of concurrent workers (builder.WithThreads(threads))
// partSize: optional; if 0 uses downloader defaults.
func DownloadParallel(ctx context.Context, d *downloader.Downloader, client downloader.Client, loc tg.InputFileLocationClass, size int64, outPath string, threads int, partSize int, cb ProgressFunc) error {
	if d == nil || client == nil || loc == nil {
		return errors.New("missing downloader/client/loc")
	}
	// prepare file for random writes
	dirPath := filepath.Dir(outPath)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	// pre-allocate if size known (best-effort; ignore error)
	_ = f.Truncate(size)

	pwa := NewProgressWriterAt(f, size, cb)

	if partSize > 0 {
		d = d.WithPartSize(partSize)
	}

	builder := d.Download(client, loc)
	if threads > 0 {
		builder = builder.WithThreads(threads)
	}

	_, err = builder.Parallel(ctx, pwa)
	return err
}

func GetAria2DlInfo(uri string) *utils.DLInfo {
	aria2 := bot.Aria2Client
	dlInfo := &utils.DLInfo{}
	if aria2 == nil {
		dlInfo.Error = fmt.Errorf("aria2 Client is inactive")
		return dlInfo
	}
	folder, _ := filepath.Abs(config.Conf.TempDir)
	resp, err := aria2.AddURI(
		arigo.URIs(uri), &arigo.Options{
			Dir: folder,
		})
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}
	status, err := resp.TellStatus()
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}
	timeout := 90 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for dlInfo.Name == "" {
		// check context
		select {
		case <-ctx.Done():
			dlInfo.Error = fmt.Errorf("could not get download info")
			return dlInfo
		default:
		}
		time.Sleep(1 * time.Second) // small backoff
		status, err = aria2.TellStatus(status.GID)
		if err != nil {
			dlInfo.Error = fmt.Errorf("TellStatus while waiting for metadata: %w", err)
			return dlInfo
		}
		if len(status.FollowedBy) > 0 {
			status, err = aria2.TellStatus(status.FollowedBy[0])
			if err != nil {
				dlInfo.Error = fmt.Errorf("TellStatus while waiting for metadata: %w", err)
				return dlInfo
			}
		}
		if status.Status == arigo.StatusError {
			dlInfo.Error = fmt.Errorf("Aria2 Download failed: %s", status.ErrorMessage)
			return dlInfo
		}
		files := status.Files
		if len(files) == 0 {
			continue
		}
		if strings.HasPrefix(files[0].Path, "[METADATA]") {
			continue
		}
		name, _ := arigo.GetDownloadName(status)
		if strings.HasSuffix(name, ".torrent") {
			continue
		}
		if status.BitTorrent.Info.Name == "" {
			if !strings.HasPrefix(files[0].Path, status.Dir) {
				continue
			}
		}
		for _, file := range status.Files {
			relPath, _ := filepath.Rel(folder, file.Path)
			dlInfo.Files = append(dlInfo.Files, relPath)
		}
		dlInfo.Count = len(dlInfo.Files)
		dlInfo.Name = name
	}
	return dlInfo
}

func getQbitClient(temp bool) (*qbit.Client, error) {
	var port string
	if temp {
		port = strconv.Itoa(config.Conf.QBitPort2)
	} else {
		port = strconv.Itoa(config.Conf.QBitPort)
	}
	client := qbit.NewClient(qbit.Config{
		Host:          "http://localhost:" + port,
		TLSSkipVerify: true,
		Timeout:       10,
	})
	if err := client.Login(); err != nil {
		return nil, err
	}
	return client, nil
}

func getFilesFromQb(ctx context.Context, qb *qbit.Client, hash string) (files []string, err error) {
	torFiles, err := qb.GetFilesInformationCtx(ctx, hash)
	if err != nil {
		return files, err
	}
	for _, file := range *torFiles {
		files = append(files, file.Name)
	}
	return files, err
}

func GetQbitDlInfo(uri string) *utils.DLInfo {
	qb, err := getQbitClient(true)
	dlInfo := &utils.DLInfo{}
	if err != nil {
		dlInfo.Error = fmt.Errorf("aria2 Client is inactive")
		return dlInfo
	}
	folder, _ := filepath.Abs(config.Conf.TempDir)
	tag := "qb_dl" + uuid.New().String()
	options := qbit.TorrentAddOptions{
		SavePath: folder,
		Tags:     tag,
	}
	qbOptions := options.Prepare()
	qbOptions["seedingTimeLimit"] = "0"
	var ctx context.Context
	var cancel context.CancelFunc

	timeout := config.Rconf.QBitTimeout * time.Second
	if timeout == 0 {
		ctx, cancel = context.Background(), func() {}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	}
	defer cancel()
	err = qb.AddTorrentFromUrlCtx(ctx, uri, qbOptions)
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}
	torrents, err := qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{Tag: tag})
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}
	for len(torrents) == 0 {
		// check context
		select {
		case <-ctx.Done():
			dlInfo.Error = fmt.Errorf("timeout while fetching torrent info")
			return dlInfo
		default:
		}
		time.Sleep(1 * time.Second) // small backoff
		torrents, err = qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{Tag: tag})
		if err != nil {
			dlInfo.Error = fmt.Errorf("error while fetching torrent info: %w", err)
			return dlInfo
		}
	}
	for torrents[0].State == qbit.TorrentStateMetaDl {
		select {
		case <-ctx.Done():
			dlInfo.Error = fmt.Errorf("timeout while fetching torrent metadata")
			return dlInfo
		default:
		}
		time.Sleep(1 * time.Second) // small backoff
		torrents, err = qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{Tag: tag})
		if err != nil {
			dlInfo.Error = fmt.Errorf("error while fetching torrent metadata: %w", err)
			return dlInfo
		}
	}
	if torrents[0].State == qbit.TorrentStateError {
		dlInfo.Error = fmt.Errorf("an unknown error occured")
		return dlInfo
	}
	dlInfo.Hash = torrents[0].Hash
	err = qb.PauseCtx(ctx, []string{dlInfo.Hash})
	if err != nil {
		dlInfo.Error = fmt.Errorf("error while pausing to fetch torrent files: %w", err)
		return dlInfo
	}
	dlInfo.Files, err = getFilesFromQb(ctx, qb, dlInfo.Hash)
	if err != nil {
		dlInfo.Error = fmt.Errorf("error while fetching torrent files: %w", err)
		return dlInfo
	}
	dlInfo.Count = len(dlInfo.Files)
	/*if dlInfo.Count > 1 {
		dlInfo.Name = torrents[0].Name
	} else if dlInfo.Count == 1 {
		dlInfo.Name = filepath.Base(torrents[0].ContentPath)
	}*/
	dlInfo.Name = torrents[0].Name
	err = qb.DeleteTagsCtx(ctx, []string{tag})
	if err != nil {
		dlInfo.Error = fmt.Errorf("error cleaning up after fetching torrent files: %w", err)
		return dlInfo
	}
	err = qb.DeleteTorrentsCtx(ctx, []string{dlInfo.Hash}, true)
	if err != nil {
		dlInfo.Error = fmt.Errorf("error cleaning up after fetching torrent files: %w", err)
		return dlInfo
	}
	return dlInfo
}

func CleanAllQbit(ctx context.Context) {
	if !bot.SQS {
		return
	}
	qb, err := getQbitClient(false)
	if err != nil {
		return
	}
	torrents, err := qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{})
	if err != nil {
		return
	}
	var hashes []string
	for _, tor := range torrents {
		hashes = append(hashes, tor.Hash)
	}
	if len(hashes) == 0 {
		return
	}
	_ = qb.RemoveTagsCtx(ctx, hashes, "")
	_ = qb.DeleteTorrentsCtx(ctx, hashes, true)
}

func GetMegaDlInfo(uri string) *utils.DLInfo {
	dlInfo := &utils.DLInfo{}
	files, err := bot.MegaClient.ListFilesFromLink(uri)
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}

	for _, file := range files {
		dlInfo.Files = append(dlInfo.Files, file.Name)
	}
	dlInfo.Count = len(dlInfo.Files)
	if dlInfo.Count == 1 {
		dlInfo.Name = dlInfo.Files[0]
	} else {
		dlInfo.Name = files[0].Parent
	}
	return dlInfo
}

func GetYdlInfo(uri string) *utils.DLInfo {
	dlInfo := &utils.DLInfo{}
	dl := ytdlp.New().Cookies(".cookies.txt").PrintJSON() // DumpJSON()
	var ctx context.Context
	var cancel context.CancelFunc

	timeout := config.Rconf.YtdlpTimeout * time.Second
	if timeout == 0 {
		ctx, cancel = context.Background(), func() {}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	}
	defer cancel()

	res, err := dl.Run(ctx, uri)
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}
	infos, err := res.GetExtractedInfo()
	if err != nil {
		dlInfo.Error = err
		return dlInfo
	}

	if len(infos) == 0 {
		dlInfo.Error = fmt.Errorf("no videos available to download")
		return dlInfo
	}
	info := infos[0]
	if info.Title == nil {
		dlInfo.Error = fmt.Errorf("extractor failed to fetch any videos")
		return dlInfo
	}
	if *info.Title == "" {
		n := "Unknown Title"
		info.Title = &n
	}
	dlInfo.Name = *info.Title
	if len(info.Entries) == 0 {
		dlInfo.Files = append(dlInfo.Files, *info.Title)
		dlInfo.Count = 1
		return dlInfo
	}
	for _, vid := range info.Entries {
		name := vid.Title
		if name == nil {
			continue
		}
		if *name == "" {
			n := "Unknown Title"
			name = &n
		}
		dlInfo.Files = append(dlInfo.Files, *name)
	}
	dlInfo.Count = len(dlInfo.Files)
	return dlInfo
}
