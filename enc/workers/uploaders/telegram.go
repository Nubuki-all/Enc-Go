package uploaders

import (
	"context"
	"fmt"

	//"io"
	"math"
	"path/filepath"

	//"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	// "github.com/gotd/td/tgerr"

	"enc/config"
	"enc/internal/bot/types"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

type TgUpload struct {
	*types.TransferInfo
	msg    *tele.Message
	reply  *tele.Message
	upload *uploader.Uploader
	pchan  chan uploader.ProgressState
	media  tg.InputMediaClass // uploaded Media
	// err       error
	// completed bool
	forceDoc bool
	ufstr    string
	fstr     string
	dir      string
	name     string
	caption  string
	thumb    string
	action   string
	threads  int
	bytes    []byte
}

func NewTgUl(name, caption, thumb string, forceDocument bool, msg, reply *tele.Message, tInfo *types.TransferInfo) *TgUpload {
	conf := config.Conf
	return &TgUpload{
		TransferInfo: tInfo,
		msg:          msg,
		name:         name,
		caption:      caption,
		reply:        reply,
		thumb:        thumb,
		threads:      3,
		upload:       uploader.NewUploader(msg.Ctx.Raw),
		forceDoc:     forceDocument,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
	}
}

func (ul *TgUpload) Start() (err error) {
	// configure
	_ = ul.upload.WithProgress(ul)
	_ = ul.upload.WithThreads(ul.threads)
	_ = ul.upload.WithPartSize(524288)
	ul.action = "upload"
	asDoc := config.Conf.UploadAsDoc
	if ul.forceDoc {
		asDoc = true
	}
	spoiler := false
	if !asDoc {
		spoiler = config.Conf.UVS
		ul.action = "upload-vid"
	}

	rconf := config.Rconf
	ulTimeout := rconf.TgUlTimeout * time.Second

	var ctx context.Context
	var cancel context.CancelFunc

	if ulTimeout == 0 {
		ctx, cancel = context.Background(), func() {}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), ulTimeout)
	}

	defer cancel()

	doneCh := make(chan struct{})
	ul.pchan = make(chan uploader.ProgressState, 1)
	start := time.Now()
	var wg sync.WaitGroup
	progressInterval := config.Rconf.ProgresUpdateInterval * time.Second
	actionInterval := 5 * time.Second

	wg.Add(1)
	go func() {
		ticker := time.NewTicker(progressInterval)
		aTicker := time.NewTicker(actionInterval)
		defer ticker.Stop()
		defer aTicker.Stop()
		defer wg.Done()

		var stored *uploader.ProgressState
		var stored2 uploader.ProgressState
	Loop:
		for {
			select {
			case <-ctx.Done():
				break Loop
			case ev := <-ul.pchan:
				ul.ProgressCallback(start, &ev)
				break Loop
			case <-doneCh:
				break Loop
			}
		}

		for {
			select {
			case <-ctx.Done():
				// timeout
				return
			case ev := <-ul.pchan:
				stored2 = ev
				stored = &ev
			case <-aTicker.C:
				if stored2.Uploaded != 0 {
					ul.actionCallback(stored2)
				}
			case <-ticker.C:
				if stored != nil {
					ul.ProgressCallback(start, stored)
				}
				stored = nil
			case <-doneCh:
				return
			}
		}
	}()

	var duration int
	var inputFile, thumbFile tg.InputFileClass
	var mime *mimetype.MIME
	defer func() { _ = ul.msg.Action("", 0) }()
	if len(ul.bytes) == 0 {
		mime, _ = mimetype.DetectFile(ul.Path)
		ul.dir = filepath.Dir(ul.Path)
		inputFile, err = ul.upload.FromPath(ctx, ul.Path)
	} else {
		mime = mimetype.Detect(ul.bytes)
		inputFile, err = ul.upload.FromBytes(ctx, ul.name, ul.bytes)
	}
	close(doneCh)
	wg.Wait()
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "engine forcibly closed") ||
			strings.Contains(errStr, "broken pipe") ||
			strings.Contains(errStr, "EOF") {
			ul.waitTillReconnect()
			return ErrRetry
		}
		if !ul.IsCancelled {
			return err
		} else {
			return nil
		}
	}
	attributes := []tg.DocumentAttributeClass{
		&tg.DocumentAttributeFilename{FileName: ul.name},
	}
	if !asDoc {
		thumbFile, duration = ul.getThumbWithDuration(ctx, mime.Extension())
		attributes = append(attributes, &tg.DocumentAttributeVideo{
			SupportsStreaming: true,
			Duration:          float64(duration),
		})
	} else {
		thumbFile = ul.uploadThumbnail(ctx)
	}
	ul.media = &tg.InputMediaUploadedDocument{
		File:       inputFile,
		MimeType:   mime.String(),
		ForceFile:  asDoc,
		Spoiler:    spoiler,
		Thumb:      thumbFile,
		Attributes: attributes,
	}
	return ul.sendMedia()
}

func (ul *TgUpload) actionCallback(upload uploader.ProgressState) {
	total := upload.Total
	current := upload.Uploaded
	var percentage float64
	if total > 0 {
		percentage = float64(current * 100 / total)
	}
	_ = ul.msg.Action(ul.action, int(percentage))
}

func (ul *TgUpload) ProgressCallback(start time.Time, upload *uploader.ProgressState) {
	total := upload.Total
	current := upload.Uploaded
	remainingSize := total - current
	diff := time.Since(start).Round(time.Second).Seconds()
	var speed int64
	if diff > 0 {
		speed = current / int64(diff)
	}
	var percentage float64
	var timeToCompletion string

	elapsedTime := utils.TimeFormatter(diff)
	if speed > 0 {
		timeToCompletion = utils.TimeFormatter(float64(remainingSize) / float64(speed))
	}
	if total > 0 {
		percentage = float64(current * 100 / total)
	}
	//_ = ul.msg.Action(ul.action, int(percentage))
	filled := int(math.Floor(percentage / 10.0))
	finStr := ul.fstr
	unfinStr := ul.ufstr
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
	if ul.dir != "" {
		msgHead = "**From folder:** `" + ul.dir + "`\n"
	}
	msgHead += "**File:** `" + upload.Name + "`"
	msgHead = config.Conf.CapDeco + " **Uploading…**\n" + msgHead
	txt := msgHead + "\n" + tmp
	_, _ = ul.msg.EditText(txt)
}

func (ul *TgUpload) Chunk(ctx context.Context, state uploader.ProgressState) error {
	if ul.IsCancelled {
		return fmt.Errorf("upload has been cancelled")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("upload timed out")
	case ul.pchan <- state:
		// sent successfully
	default:
		// channel full — drop the old and put the new one so latest wins
		select {
		case <-ul.pchan:
		default:
		}
		select {
		case ul.pchan <- state:
		default:
			// if still can't send (shouldn't happen), just drop it
		}
	}
	return nil
}

func (ul *TgUpload) getThumbWithDuration(ctx context.Context, ext string) (thumb tg.InputFileClass, dur int) {
	var path string
	if len(ul.bytes) > 0 {
		tmp := &utils.TempFile{}
		if err := tmp.FromBytes(ext, ul.bytes); err != nil {
			logger.Error(err)
		}
		path = tmp.Name
		defer func() {
			if err := tmp.Remove(); err != nil {
				logger.Warn(err)
			}
		}()
	} else {
		path = ul.Path
	}
	tmp := &utils.TempFile{}
	if err := tmp.Create(".jpg"); err != nil {
		logger.Error(err)
	}
	defer func() {
		if err := tmp.Remove(); err != nil {
			logger.Warn(err)
		}
	}()
	thumbPath, dur, err := utils.GetVideoThumbnailAndDuration(path, tmp.Name)
	if err != nil {
		logger.Error(err)
		return thumb, dur
	} else if thumbPath == "" {
		logger.Warn("Could not get video thumbnail!")
		return thumb, dur
	}
	uClient := uploader.NewUploader(ul.msg.Ctx.Raw)
	thumb, err = uClient.FromPath(ctx, thumbPath)
	if err != nil {
		logger.Error("Uploading thumbnail to telegram server failed with: ", err)
		return thumb, dur
	}
	return thumb, dur
}

func (ul *TgUpload) uploadThumbnail(ctx context.Context) (thumb tg.InputFileClass) {
	if ul.thumb == "" {
		return thumb
	}
	uClient := uploader.NewUploader(ul.msg.Ctx.Raw)
	thumb, err := uClient.FromPath(ctx, ul.thumb)
	if err != nil {
		logger.Error("Uploading thumbnail to telegram server failed with: ", err)
		return thumb
	}
	return thumb
}

func (ul *TgUpload) sendMedia() (err error) {
	if ul.reply != nil {
		_, err = ul.reply.ReplyMediaDoc(ul.media,
			&tele.UploadOptions{
				Quote:   true,
				Caption: ul.caption,
			},
		)
	} else {
		_, err = ul.msg.Edit(ul.caption,
			&tele.EditOptions{Media: ul.media},
		)
	}
	return err
}

func (ul *TgUpload) waitTillReconnect() {
	timeoutDuration := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()
	retryInterval := 5 * time.Second
	logger.Info("Attempting to wait till reconnection.")
	for {
		err := telegram.Client.Ping(ctx)
		if err == nil {
			fmt.Println("successfully waited for restart.")
			return
		}

		fmt.Printf("Attempt failed: %v. Retrying in %v...\n", err, retryInterval)

		// Try to wait for either the next retry interval
		// or the overall timeout.
		select {
		case <-time.After(retryInterval):

			continue
		case <-ctx.Done():

			fmt.Printf("Total duration exceeded (%s). Giving up.\n", timeoutDuration)
			return
		}
	}
}
