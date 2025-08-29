package downloaders

import (
	"context"
	"errors"
	//"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

var ErrDownloadCanceled = errors.New("download canceled by user")

// ProgressFunc is called on progress events. start is the download start time,
// done is bytes downloaded so far, total is total size (0 if unknown).
type ProgressFunc func(start time.Time, done, total int64, w io.Writer, wa io.WriterAt)

// ================= progress writers ======================

// progressWriter is an io.Writer that counts bytes and calls cb(start, done, total).
type progressWriter struct {
	w       io.Writer
	start   time.Time
	Cancel bool
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
	Cancel bool
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
		new := atomic.AddInt64(&p.written, int64(n))
		if p.cb != nil {
			p.cb(p.start, new, p.total, nil, p)
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
	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

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
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

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
