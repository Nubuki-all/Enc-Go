package downloaders

import (
	"context"
	"fmt"
	//"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

type TgUpload struct {
	*types.TransferInfo
	msg       *tele.Message
	reply     *tele.Message
	upload    *uploader.Uploader
	pchan     chan *uploader.ProgressState
	err       error
	completed bool
	forceDoc  bool
	quote     bool
	ufstr     string
	fstr      string
	dir       string
	name      string
	thumb     string
	action    string
	bytes     []byte
}

func NewTgUl(name, thumb string, forceDocument bool, msg, reply *tele.Message, tInfo *types.TransferInfo) *TgUpload {
	conf := config.Conf
	return &TgUpload{
		TransferInfo: tInfo,
		msg:          msg,
		name:         name,
		reply:        reply,
		thumb:        thumb,
		upload:       uploader.NewUploader(msg.Ctx.Raw),
		forceDoc:     forceDocument,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
	}
}

func (ul *TgUpload) Start() (err error) {
	// configure
	_ = ul.upload.WithProgress(ul)
	_ = ul.upload.WithThreads(4)
	ul.action = "upload"
	asDoc := config.Conf.UploadAsDoc
	if ul.ForceFile {
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

	done := make(chan error, 1)
	ul.pchan = make(chan *uploaders.ProgressState, 1)
	start := time.Now()
	progressInterval := config.Rconf.ProgresUpdateInterval * time.Second

	go func() {
		ticker := time.NewTicker(progressInterval)
		defer ticker.Stop()

		var stored *uploaders.ProgressState
		for {
			select {
			case <-ctx.Done():
				done <- fmt.Errorf("upload timed out after %s", utils.TimeFormatter(dlTimeout.Seconds()))
				return
			case ev := <-ul.pchan:
				ul.ProgressCallback(start, ev)
				return
			case <-doneCh:
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				// timeout or external cancel
				done <- fmt.Errorf("upload timed out after %s", utils.TimeFormatter(dlTimeout.Seconds()))
				return
			case ev := <-ul.pchan:
				stored = ev
			case <-ticker.C:
				if stored != nil {
					ul.ProgressCallback(start, stored)
				}
			case <-doneCh:
				return
			}
		}
	}()

	var dhuration int
	var inputFile, thumbFile tg.InputFileClass
	var mime mimetype.MIME
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
	if err != nil {
		return err
	}
	attributes := []tg.DocumentAttributeClass{
		&tg.DocumentAttributeFilename{FileName: ul.name},
	}
	if !asDoc {
		thumbFile, duration = ul.getThumbWithDuration(ctx, mime.Extension())
		attributes = append(attributes, &tg.DocumentAttributeVideo{
			SupportsStreaming: true,
			Duration:          duration,
		})
	} else {
		thumbFile = ul.uploadThumbnail(ctx)
	}
	media := &tg.InputMediaUploadedDocument{
		File:       inputFile,
		MimeType:   mime.String(),
		ForceFile:  asDoc,
		Spoiler:    spoiler,
		Thumb:      thumbFile,
		Attributes: attributes,
	}
	return ul.sendMedia(media)
}

func (ul *TgUpload) ProgressCallback(start time.Time, upload *uploaders.ProgressState) {
	total := upload.Total
	current := upload.Uploaded
	remainingSize := total - current
	diff := time.Since(start).Round(time.Second).Seconds()
	speed := done / int64(diff)
	eta := torrent.ETA
	var percentage float64
	var timeToCompletion string

	elapsedTime := utils.TimeFormatter(diff)
	if speed > 0 {
		timeToCompletion = utils.TimeFormatter(float64(remainingSize) / float64(speed))
	}
	if total > 0 {
		percentage = float64(current * 100 / total)
	}
	_ = ul.msg.Action(ul.action, int(percentage))
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
	if dl.dir != "" {
		msgHead = "**From folder:** `" + dl.dir + "`\n"
	}
	msgHead += "**File:** `" + upload.Name + "`"
	msgHead = conf.Config.CapDeco + " **Uploading…**\n" + msgHead
	txt := msgHead + "\n" + tmp
	_, _ = ul.msg.EditText(txt)
	return ok
}

func (ul *TgUpload) Chunk(ctx context.Context, state uploader.ProgressState) error {
	if ul.IsCancelled {
		return fmt.Errorf("upload has been cancelled")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("upload timed out")
	case ul.pchan <- &state:
		// sent successfully
	default:
		// channel full — drop the old and put the new one so latest wins
		select {
		case <-ul.pchan:
		default:
		}
		select {
		case ul.pchan <- &state:
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
		tmp.FromBytes(ext, ul.bytes)
		path = tmp.Name
		defer tmp.Remove()
	} else {
		path = ul.Path
	}
	tmp := &utils.TempFile{}
	tmp.Create(".jpg")
	defer tmp.Remove()
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
	uClient := uploader.NewUploader(ul.msg.Ctx.Raw)
	thumb, err = uClient.FromPath(ctx, ul.thumb)
	if err != nil {
		logger.Error("Uploading thumbnail to telegram server failed with: ", err)
		return thumb
	}
	return thumb
}

func (ul *TgUpload) sendMedia(media *tg.InputMediaUploadedDocument) (err error) {
	if ul.reply != nil {
		_, err = ul.reply.ReplyMediaDoc(media,
			&tele.UploadOptions{
				Quote:   true,
				Caption: ul.caption,
			},
		)
	} else {
		_, err = ul.msg.Edit(ul.caption,
			&tele.EditOptions{Media: media},
		)
	}
	return err
}
