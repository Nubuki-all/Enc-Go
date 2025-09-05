package downloaders

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/siku2/arigo"
	
	"enc/config"
	"enc/internal/bot"
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
			dlInfo.Error = fmt.Errorf("could not fetch selected torrent info")
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

