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

func (dl *TgDownload) Start() error {
	ctx := dl.msg.Ctx
	return DownloadParallel(
		ctx,
		dl.downloader,
		ctx.Raw,
		dl.Location,
		dl.Size,
		dl.Path,
		dl.threads,
		dl.partSize,
		dl.ProgressCallback,
	)
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
	progress := fmt.Sprintf("```%s```\n**Progress:** `%.2f%%`\n", bar, percentage)
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
