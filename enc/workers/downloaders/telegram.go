package downloaders

import (
	"fmt"
	//"io"
	"math"
	"strings"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot/types"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

type TgDownload struct {
	*types.TransferInfo
	Location   tg.InputFileLocationClass
	Size       int64
	msg        *tele.Message
	downloader *downloader.Downloader
	threads    int
	partSize   int
	ufstr      string
	fstr       string
}

func NewTgDl(location tg.InputFileLocationClass, size int64, msg *tele.Message, tInfo *types.TransferInfo) *TgDownload {
	conf := config.Conf
	return &TgDownload{
		TransferInfo: tInfo,
		Location:     location,
		Size:         size,
		msg:          msg,
		downloader:   downloader.NewDownloader(),
		threads:      4,
		partSize:     0,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
	}
}

// progressEvent carries a single event (same fields you already pass to dl.ProgressCallback).
type progressEvent struct {
	start time.Time
	done  int64
	total int64
	pw    *progressWriter
	pwa   *progressWriterAt
}

// Start uses a channel-based throttler around dl.ProgressCallback.
// First event is handled immediately; subsequent events are throttled to `interval`.
func (dl *TgDownload) Start() error {
	ctx := dl.msg.Ctx
	interval := 10 * time.Second

	// single-slot buffered channel: holds latest event.
	ch := make(chan progressEvent, 1)
	doneCh := make(chan struct{})
	var wg sync.WaitGroup

	// Consumer goroutine: calls dl.ProgressCallback immediately for the first event,
	// then at most once per interval after that.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var lastCall time.Time    // zero -> no call yet
		var stored *progressEvent // latest event queued while waiting for interval

		for {
			select {
			case ev := <-ch:
				now := time.Now()
				// If no call has happened yet -> immediate first run.
				// Otherwise, if enough time passed since lastCall -> immediate run.
				if lastCall.IsZero() || now.Sub(lastCall) >= interval {
					// Call in goroutine so consumer stays responsive.
					go dl.ProgressCallback(ev.start, ev.done, ev.total, ev.pw, ev.pwa)
					lastCall = now
				} else {
					// Store latest event to be executed on next tick (latest wins).
					stored = &ev
				}

			case <-ticker.C:
				// On each tick, if we have a stored event, call it now.
				if stored != nil {
					ev := *stored
					go dl.ProgressCallback(ev.start, ev.done, ev.total, ev.pw, ev.pwa)
					lastCall = time.Now()
					stored = nil
				}

			case <-doneCh:
				// flush last stored event synchronously so final update happens before exit
				if stored != nil {
					ev := *stored
					dl.ProgressCallback(ev.start, ev.done, ev.total, ev.pw, ev.pwa)
					stored = nil
				}
				return
			}
		}
	}()

	// The progressFunc we hand to the downloader (hot path). It's non-blocking and
	// replaces older queued values when overloaded.
	progressFunc := func(start time.Time, done, total int64, w *progressWriter, wa *progressWriterAt) {
		ev := progressEvent{start: start, done: done, total: total, pw: w, pwa: wa}

		// Try fast non-blocking send; if channel is full, remove the old value and replace with new
		select {
		case ch <- ev:
			// sent successfully
		default:
			// channel full — drop the old and put the new one so latest wins
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
				// if still can't send (shouldn't happen), just drop it
			}
		}
	}

	// call the downloader with your progressFunc
	err := DownloadParallel(
		ctx,
		dl.downloader,
		ctx.Raw,
		dl.Location,
		dl.Size,
		dl.Path,
		dl.threads,
		dl.partSize,
		progressFunc,
	)

	// signal consumer to finish and wait for it (this flushes the last event)
	close(doneCh)
	wg.Wait()

	return err
}

func (dl *TgDownload) ProgressCallback(start time.Time, done, total int64, w *progressWriter, wa *progressWriterAt) {
	if dl.PauseProgress {
		return
	}
	if dl.IsCancelled {
		if w != nil {
			w.Cancel = true
		} else if wa != nil {
			wa.Cancel = true
		}
		// return
	}
	diff := time.Since(start).Round(time.Second).Seconds()
	if total < done {
		return
	}
	percentage := float64(done * 100 / total)
	speed := done / int64(diff)
	elapsedTime := utils.TimeFormatter(diff)
	timeToCompletion := utils.TimeFormatter(float64(total-done) / float64(speed))
	filled := int(math.Floor(percentage / 10.0))
	finStr := dl.fstr
	unfinStr := dl.ufstr
	if finStr == "" {
		finStr = random.EnHearts()
	}
	bar := strings.Repeat(finStr, filled) + strings.Repeat(unfinStr, 10-filled)
	progress := fmt.Sprintf("```\n%s```\n**Progress:** `%.2f%%`\n", bar, percentage)
	// ensure fallbacks for time strings
	if timeToCompletion == "" {
		timeToCompletion = "0 s"
	}
	if elapsedTime == "" {
		elapsedTime = "0 s"
	}
	tmp := fmt.Sprintf(
		"%s`%s of %s`\n**Speed:** `%s/s`\n**ETA:** `%s`\n**Elapsed:** `%s`\n",
		progress,
		utils.HBS(done),  // e.g. "1.2 MiB"
		utils.HBS(total), // e.g. "10.0 MiB"
		utils.HBS(speed), // e.g. "100 KiB"
		timeToCompletion,
		elapsedTime,
	)
	txt := "`Downloading to` `" + dl.Path + "`…\n" + tmp
	_, _ = dl.msg.EditText(txt)
}
