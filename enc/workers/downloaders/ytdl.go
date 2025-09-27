package downloaders

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"strconv"
	"time"

	"github.com/lrstanley/go-ytdlp"

	"enc/config"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

type YtdlDownload struct {
	*types.TransferInfo
	Size int64
	msg  *tele.Message
	done bool
	// timeout      bool
	ufstr    string
	fstr     string
	basePath string
	name     string
	selected string
	dir      string
	uri      string
}

func NewYTDl(uri, dir, selected string, msg *tele.Message, tInfo *types.TransferInfo) *YtdlDownload {
	conf := config.Conf
	return &YtdlDownload{
		TransferInfo: tInfo,
		msg:          msg,
		dir:          dir,
		selected:     selected,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
		uri:          uri,
	}
}

func (dl *YtdlDownload) Start() (err error) {
	rconf := config.Rconf
	dlTimeout := rconf.YDLTimeout * time.Second
	progressInterval := rconf.ProgresUpdateInterval * time.Second
	updateInterval := 1 * time.Second

	var ctx context.Context
	var cancel context.CancelFunc

	if dlTimeout == 0 {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), dlTimeout)
	}

	defer cancel()

	dlInfo := GetYdlInfo(dl.uri)
	if dlInfo.Error != nil {
		return dlInfo.Error
	}
	p, err := filepath.Abs(".bin")
	if err != nil {
		return err
	}
	dl.name = dlInfo.Name
	dl.basePath = filepath.Join(dl.dir, dlInfo.Name)
	dl.Path = dl.basePath
	ytdl := ytdlp.New().Cookies(".cookies.txt").SubLangs("all").EmbedSubs().EmbedThumbnail().
		FFmpegLocation(p).
		EmbedMetadata().
		EmbedChapters().
		Quiet().
		Output(dl.Path + "%(title)s.mkv")
	if dlInfo.Count > 0 {
		if dl.selected != "" {
			s, _ := strconv.Atoi(dl.selected)
			if s == 0 || s > len(dlInfo.Files) {
				return fmt.Errorf("ytdl: file_id not found")
			}
			ytdl = ytdl.PlaylistItems(dl.selected)
			dl.Path = filepath.Join(dl.Path, dlInfo.Files[s-1]+".mkv")
		}
	} else {
		dl.Path = filepath.Join(dl.Path, dlInfo.Name+".mkv")
	}
	go func() {
		uTicker := time.NewTicker(updateInterval)
		defer uTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-uTicker.C:
				if dl.IsCancelled {
					cancel()
					return
				} else if dl.done {
					return
				}
			}
			// continue loop
		}
	}()
	ytdl = ytdl.ProgressFunc(progressInterval, dl.ProgressCallback)
	_, err = ytdl.Run(ctx, dl.uri)
	dl.done = true
	if err != nil && !dl.IsCancelled {
		return err
	}
	return nil
}

func (dl *YtdlDownload) ProgressCallback(update ytdlp.ProgressUpdate) {
	if dl.PauseProgress {
		return
	}

	total := int64(update.TotalBytes)
	current := int64(update.DownloadedBytes)
	remainingSize := total - current
	diff := time.Since(update.Started).Round(time.Second).Seconds()
	var speed int64
	if diff > 0 {
		speed = current / int64(diff)
	}

	var percentage float64
	var timeToCompletion string
	if speed > 0 {
		timeToCompletion = utils.TimeFormatter(float64(remainingSize) / float64(speed))
	}
	if total > 0 {
		percentage = float64(current * 100 / total)
	}
	elapsedTime := utils.TimeFormatter(diff)
	filled := int(math.Floor(percentage / 10.0))
	finStr := dl.fstr
	unfinStr := dl.ufstr
	if finStr == "" {
		finStr = random.EnHearts()
	}
	bar := strings.Repeat(finStr, filled) + strings.Repeat(unfinStr, 10-filled)
	progress := fmt.Sprintf("```\n%s```\n**Progress:** `%.2f%%`\n", bar, percentage)
	if timeToCompletion == "" {
		timeToCompletion = "0 s"
	}
	if elapsedTime == "" {
		elapsedTime = "0 s"
	}
	tmp := fmt.Sprintf(
		"%s`%s of %s`\n**Speed:** `%s/s`\n**Remains:** `%s`\n**ETA:** `%s`\n**Elapsed:** `%s`\n",
		progress,
		utils.VHBS(current),
		utils.VHBS(total),
		utils.VHBS(speed),
		utils.VHBS(remainingSize),
		timeToCompletion,
		elapsedTime,
	)
	var msgHead string
	switch update.Status {
	case ytdlp.ProgressStatusStarting:
		msgHead = "`Starting Download...`"
	case ytdlp.ProgressStatusPostProcessing:
		msgHead = "`Processing Download...`"
	case ytdlp.ProgressStatusError:
		msgHead = "`Download error.`"
	case ytdlp.ProgressStatusDownloading:
		msgHead = "**Downloading:**\n`" + update.Filename + "`"
		msgHead += "\n**via:** yt-dlp."
	case ytdlp.ProgressStatusFinished:
		msgHead = "**.................**"
	default:
		msgHead = "`..................`"
		logger.Info(update.Status) // debug
	}
	txt := msgHead + "\n" + tmp
	_, _ = dl.msg.EditText(txt)
}

func (dl *YtdlDownload) Clean() {
	if !dl.done {
		logger.WarnW("yt-dlp Dl: Calling Clean on a download that exited early")
		return
	}
	_ = os.RemoveAll(dl.basePath)
}
