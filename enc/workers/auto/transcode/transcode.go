// Package transcode handles automatically encoding queued items.
package transcode

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"enc/config"
	"enc/internal/bot"

	"enc/internal/bot/types"
	"enc/internal/clients/telegram"
	"enc/internal/db"
	"enc/internal/logger"
	"enc/utils"
	batchutils "enc/utils/batch"
	"enc/utils/parser"
	"enc/utils/tele"
	"enc/workers/encoders"
	"enc/workers/uploaders"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	Einfo       = utils.NewEncodeInfo()
	workerMutex sync.Locker
)

// Start starts the encode workers in the background.
// To be called only once
func Start() {
	if config.Conf.SequentialJobs {
		workerMutex = &sync.Mutex{}
	} else {
		workerMutex = &types.NoopMutex{}
	}
	go something()
}

func something() { // couldn't find a better name
	for {
		if bot.Paused.Any() {
			time.Sleep(5 * time.Second)
			continue
		}
		entry, found := bot.Queue.GetByIndex(0)
		if !found {
			time.Sleep(5 * time.Second)
			continue
		}
		logger.InfoW("Handling queued item", zap.String("Name", entry.Name))
		thing(entry)
		db.SaveQueue(telegram.Context)
	}
}

func thing(entry types.QueueEntry) {
	batchItem := batchutils.GetNextBatchItem(entry.Batch)
	if len(entry.Batch) != 0 && batchItem == nil {
		// No item to encode in batch; skip
		_, _ = bot.Queue.Pop(entry.Name)
		return
	}
	if batchItem != nil {
		Einfo.BatchID = batchItem.ID
	}
	fchannel := config.Conf.FChannel
	fmsgID := config.Conf.FChannelStat
	fBanner := config.Conf.FBanner
	LogChat := config.Conf.LogChannel
	var Bmsg, Lmsg *tele.Message
	var err error
	Bmsg = sendBanner(fchannel, fmsgID, fBanner)
	msg, err := tele.SendMsg(
		entry.ChatID,
		"`Waiting for download handler…`",
		&tele.SendOptions{ReplyTo: entry.MessageID},
	)
	if err != nil {
		broadcastAndPauseOnError()
		return
	}
	if LogChat == msg.ChatID {
		LogChat = 0
	}
	if LogChat != 0 {
		Lmsg, err = tele.SendMsg(
			LogChat,
			"Starting download...",
			nil,
		)
		if err != nil {
			LogChat = 0
		}
	}
	download := getDownload(entry, batchItem, msg, Lmsg)
	if download == nil {
		_, _ = bot.Queue.Pop(entry.Name)
		return
	}

	ID := strconv.FormatInt(msg.ChatID, 10) + ":" + strconv.Itoa(msg.ID)
	if download.Background {
		err = download.Attach(ID, entry.Sender)
	} else {
		download.Init(ID, entry.Sender)
		err = download.Start()
	}
	if err != nil || download.IsCancelled {
		_, _ = download.ReportStatus()
		_, _ = bot.Queue.Pop(entry.Name)
		return
	}
	downloadTime := utils.TimeFormatter(time.Since(download.Time).Round(time.Second).Seconds())
	if Lmsg != nil {
		_ = Lmsg.Delete()
	}
	job := &utils.EncodeJob{}
	jobs := job.PendingInfo()
	SourceMInfo, _ := utils.PostMediaInfo(download.Path)
	dlName := filepath.Base(download.Path)
	go func() {
		err := uploaders.DumpDl(download.Path, dlName, entry.Sender, msg)
		if err != nil {
			logger.WarnW("[Dumper]: An error occured", zap.Error(err))
		}
	}()
	Einfo.Current = dlName
	pName, err := parser.QParse(dlName, &parser.ParseOptions{
		Direct:         entry.ForceName,
		UseAniList:     entry.AniList,
		FilterOverride: entry.Filter,
		Version:        entry.Version,
	})
	if err == nil {
		Einfo.Current = pName
	}
	if !config.Conf.NoBanner {
		txt, url, _ := parser.FPost(
			dlName,
			&parser.FPostOptions{
				Path:         download.Path,
				Anilist:      entry.AniList,
				Fcodec:       config.Conf.FCodec,
				MediaInfoURL: SourceMInfo,
				Job:          *job,
			},
		)
		if txt != url && Bmsg != nil {
			_, _ = Bmsg.Edit(txt, &tele.EditOptions{Media: &tg.InputMediaPhotoExternal{URL: url}})
		} else if txt != url {
			_, _ = tele.SendMedia(fchannel, &tg.InputMediaPhotoExternal{URL: url}, &tele.SendOptions{Caption: txt})
		}

	}
	currentName, err := parser.QParseT(entry.Name, entry.Version, entry.Filter)
	if err != nil {
		currentName = entry.Name
	}
	if Einfo.Previous != currentName {
		Einfo.Previous = currentName
		if LogChat := config.Conf.LogChannel; LogChat != 0 {
			for _, char := range []string{"!", ":", ";", "-", " "} {
				currentName = strings.ReplaceAll(currentName, char, "_")
			}
			_ = tele.SendMessage(LogChat, "#"+currentName)
		}
	}
	if config.Conf.CacheDL {
		cacheNextDownload(bot.Queue.AsSlice(), batchItem)
	}
	g, ctx := errgroup.WithContext(context.Background())
	for i, args := range jobs.List {
		w := encodeWorker{
			ctx:       ctx,
			job:       job,
			batchItem: batchItem,
			item:      entry,
			dlName:    dlName,
			dlPath:    download.Path,
			no:        i,
			logChat:   LogChat,
			mInfo:     SourceMInfo,
			dTime:     downloadTime,
		}
		if fchannel != 0 {
			w.fmsg, _ = tele.SendMsg(fchannel, "file #"+strconv.Itoa(i), nil)
		}
		g.Go(func() error {
			return w.start(args)
		})
	}
	sendSticker(entry, batchItem)
	_ = g.Wait()
	Einfo.Reset()
	if batchItem != nil {
		updatedBatch := batchutils.MarkAsDone(entry.Batch, batchItem.ID)
		entry.Batch = updatedBatch
		bot.Queue.Update(entry)
		return
	}
	_, _ = bot.Queue.Pop(entry.Name)
}

type encodeWorker struct {
	logChat   int64
	ctx       context.Context
	fmsg      *tele.Message
	job       *utils.EncodeJob
	batchItem *types.BatchItem
	item      types.QueueEntry
	dlName    string
	dlPath    string
	mInfo     string
	dTime     string
	no        int
}

func (e *encodeWorker) start(encodeArgs string) error {
	codec := utils.GetCodec(encodeArgs)
	Einfo.WorkersCodec = append(Einfo.WorkersCodec, codec)
	Einfo.Workers++
	defer func() {
		remove(Einfo.WorkersCodec, codec)
		Einfo.Workers--
	}()
	workerMutex.Lock()
	defer workerMutex.Unlock()
	select {
	case <-e.ctx.Done():
		return nil
	default:
	}
	select {
	case Einfo.Event <- struct{}{}:
	default:
	}
	sMsg, err := tele.SendMsg(e.item.ChatID, "`Waiting for encoder worker…`", &tele.SendOptions{ReplyTo: e.item.MessageID})
	if err != nil {
		logger.Error(err)
		return nil
	}
	var Lmsg *tele.Message
	if e.logChat != 0 {
		Lmsg, _ = tele.SendMsg(e.logChat, "`Waiting for encoder worker…`", nil)
	}
	result, err := parser.ParseName(
		e.dlName,
		&parser.ParseOptions{
			Direct:         e.item.ForceName,
			UseAniList:     e.item.AniList,
			FilterOverride: e.item.Filter,
			Version:        e.item.Version,
			Path:           e.dlPath,
			EncString:      encodeArgs,
		},
	)
	if err != nil {
		result.FileName = e.dlName
		result.MediaInfoName = utils.RemoveExt(result.FileName)
	}
	fn := result.MediaInfoName
	if e.no > 1 && !config.Conf.SequentialJobs {
		fn += "-" + strconv.Itoa(e.no)
	}
	fn += filepath.Ext(result.FileName)
	outPath := filepath.Join(config.Conf.EncodeDir, fn)
	var thumbPath string
	if e.item.AniList && e.item.ForceName == "" {
		thumb := &utils.TempFile{}
		defer func() {
			if err := thumb.Remove(); err != nil {
				logger.Warn(err)
			}
		}()
		if err := thumb.Create(".jpg"); err != nil {
			logger.Warn(err)
		} else {
			thumbPath = thumb.Name
		}
	}
	r := parser.DynamicThumb(
		e.ctx,
		e.item.Name,
		thumbPath,
		(e.item.AniList && e.item.ForceName == ""),
		e.item.Filter,
	)
	if !r.Downloaded {
		thumbPath = ""
	}
	cmd := formatArgs(encodeArgs, r.Title, r.Episode, r.Season, result.MediaInfoName, e.dlPath)
	cmd = strings.ReplaceAll(cmd, "{input}", e.dlPath)
	cmd = strings.ReplaceAll(cmd, "{output}", outPath)
	id := fmt.Sprintf("%d:%d", sMsg.ChatID, sMsg.ID)
	encode := &encoders.Encoder{
		LMsg:      Lmsg,
		Msg:       sMsg,
		Job:       e.job,
		Context:   e.ctx,
		STime:     time.Now(),
		Sender:    e.item.Sender,
		ID:        id,
		ExtraPath: thumbPath,
	}
	if err := encode.Start(cmd); err != nil {
		_, _ = sMsg.Edit("Error while Starting encode: "+err.Error(), nil)
		logger.Error(err)
		return nil
	}
	encode.Callback(e.dlPath, outPath, "")
	_ = encode.AwaitCompletion()
	_ = encode.ReportStatus("")
	defer utils.SRemove(false, outPath)
	if e.job.Completed {
		return cancelAllWorkers
	}
	if !encode.Process.Success() {
		return nil
	}
	var mux *encoders.Encoder
	if bot.MuxArgs != "" {
		result2, err := parser.ParseName(
			e.item.Name,
			&parser.ParseOptions{
				Direct:         e.item.ForceName,
				UseAniList:     e.item.AniList,
				FilterOverride: e.item.Filter,
				Version:        e.item.Version,
				Path:           outPath,
			},
		)
		if err != nil {
			result = result2
		}
		fn = result2.MediaInfoName + " (Muxed)"
		if e.no > 1 && !config.Conf.SequentialJobs {
			fn += "-" + strconv.Itoa(e.no)
		}
		fn += filepath.Ext(result.FileName)
		outPath2 := filepath.Join(config.Conf.EncodeDir, fn)
		muxCmd := formatArgs(bot.MuxArgs, r.Title, r.Episode, r.Season, result.MediaInfoName, outPath)
		ffmpegCmd := fmt.Sprintf(`ffmpeg -i """%s""" %s -codec copy """%s""" -y`, outPath, muxCmd, outPath2)
		mux = &encoders.Encoder{
			Msg:    sMsg,
			LMsg:   Lmsg,
			STime:  time.Now(),
			Sender: e.item.Sender,
			ID:     id,
		}
		if err := mux.Start(ffmpegCmd); err != nil {
			_, _ = sMsg.Edit("Error while starting mux: "+err.Error(), nil)
			logger.Error(err)
			return nil
		}
		_ = mux.AwaitCompletion()
		_ = mux.ReportStatus("Muxing")
		defer utils.SRemove(false, outPath2)
		if !mux.Process.Success() {
			return nil
		}
		utils.SRemove(false, outPath)
		outPath = outPath2
	}
	if Lmsg != nil {
		_, _ = Lmsg.Edit("`Uploading…` `"+outPath+"`", nil)
	}
	cOptipns := &parser.CustCapOptions{
		Path:       outPath,
		UseAniList: e.item.AniList,
		Ver:        e.item.Version,
		Direct:     e.item.ForceName,
		Encoder:    config.Conf.Encoder,
		Filter:     e.item.Filter,
	}
	caption, err := parser.CustCap(e.item.Name, cOptipns)
	if err != nil {
		caption = fmt.Sprintf("`%s`", filepath.Base(outPath))
	}
	upload := &uploaders.Uploader{
		Msg:       sMsg,
		Caption:   caption,
		ThumbPath: thumbPath,
		FileName:  result.FileName,
	}
	upload.Init(id, outPath, e.item.Sender)
	err = upload.Start()
	if err != nil || upload.IsCancelled {
		m, e := upload.ReportStatus()
		if e != nil && Lmsg != nil {
			t, _ := m.RawText()
			t = cmp.Or(t, m.Raw.Text)
			_, _ = Lmsg.Edit(t, nil)
		}
		return nil
	}
	uploadTime := utils.TimeFormatter(
		time.Since(upload.Time).Round(time.Second).Seconds(),
	)
	_ = Lmsg.Delete()
	if e.fmsg != nil {
		go func() {
			_, _ = e.fmsg.Edit(caption, &tele.EditOptions{Media: upload.TgMedia})
		}()
	}
	text := "**Source:** `[" + cmp.Or(r.ReleaseGroup, "None") + "]`"
	if e.mInfo != "" {
		text += "\n\nMediainfo: **[(Source)](" + e.mInfo + ")**"
	}
	miMsg, _ := sMsg.Reply(text, &tele.ReplyOptions{DisableWebPreview: true})
	orgSize := utils.SizeOf(e.dlPath)
	outSize := utils.SizeOf(outPath)
	var muxText string
	if mux != nil {
		muxText = "Muxed in `" +
			utils.TimeFormatter(mux.ETime.Sub(mux.STime).Round(time.Second).Seconds()) +
			"`\n"
	}
	text = fmt.Sprintf(
		"**Encode Stats:**\n\nOriginal Size: `%s`\nEncoded Size: `%s`\nEncoded Percentage: `%.2f%%`\n\n"+
			"Downloaded in `%s`\nEncoded in `%s`\n"+
			"%sUploaded in `%s`",
		utils.HBS(orgSize),
		utils.HBS(outSize),
		100-(float64(outSize/orgSize)*100),
		e.dTime,
		utils.TimeFormatter(
			encode.ETime.Sub(encode.STime).Round(time.Second).Seconds(),
		),
		muxText,
		uploadTime,
	)
	stMsg, _ := sMsg.ReplyText(text)
	if e.logChat != 0 {
		_, _ = sMsg.Forward(e.logChat)
		if miMsg != nil {
			_, _ = miMsg.Forward(e.logChat)
		}
		if stMsg != nil {
			_, _ = stMsg.Forward(e.logChat)
		}
	}
	return nil
}
