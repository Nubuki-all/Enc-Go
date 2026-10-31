package transcode

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"enc/config"

	//"enc/internal/bot"
	"enc/internal/bot"
	"enc/internal/bot/entimer"
	"enc/internal/bot/source"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	batchutils "enc/utils/batch"
	"enc/utils/parser"
	"enc/utils/tele"
	"enc/workers/downloaders"

	"github.com/gotd/td/constant"
	"github.com/gotd/td/telegram/message/markup"
	"go.uber.org/zap"
)

const cancelAllWorkers eworkerError = "cancelled all workers"

type eworkerError string

func (e eworkerError) Error() string {
	return string(e)
}

func broadcastAndPauseOnError() {
	errorMsg := ("Due to an unknown error " +
		"bot has been paused indefinitely\n" +
		"check logs for more info.")
	mlist := utils.BcMsg(errorMsg, 0, []*tele.Message{})
	entimer.Timer.PauseIndefinitely(mlist)
}

func getDownload(entry types.QueueEntry, batchItem *types.BatchItem, msg, lmsg *tele.Message) *downloaders.Downloader {
	if config.Conf.CacheDL {
		cacheName := entry.Name
		if batchItem != nil {
			cacheName += "++" + batchItem.ID
		}
		if dl, found := downloaders.CachedDL[cacheName]; found {
			delete(downloaders.CachedDL, cacheName)
			// fill important fields
			dl.Msg = msg
			dl.LCMsg = lmsg
			return dl
		} else if len(downloaders.CachedDL) > 0 {
			logger.Warn("Cached download invalidated!")
			for _, dl := range downloaders.CachedDL {
				select {
				case <-dl.Done:
					dl.CleanDownload()
				default:
					dl.IsCancelled.Store(true)
				}
			}
			downloaders.CachedDL = make(map[string]*downloaders.Downloader, 1)
		}
	}
	download := &downloaders.Downloader{
		Msg:      msg,
		LCMsg:    lmsg,
		DlInfo:   true,
		Folder:   config.Conf.DownloadDir,
		Selected: entry.Source.Select,
		URI:      entry.URI,
		FileName: entry.Name,
	}
	switch entry.Source.Source {
	case source.Telegram:
		if entry.Doc == nil {
			// this should never ordinarily happen but just in case
			logger.WarnW(
				"Queued item from telegram is missing file meta",
				zap.String("Name", entry.Name),
			)
			return nil
		}
		loc, err := utils.BuildInputFileLocation(entry.Doc)
		if err != nil {
			logger.ErrorW(
				"Could not retrieve input file location from queued item",
				zap.Error(err),
			)
			return nil
		}
		download.InputLoc = loc
		download.Size = entry.Doc.Size
	case source.Qbit:
		download.Qbit = true
	case source.Ytdl:
		download.Ytdl = true
	}
	if batchItem != nil {
		download.Selected = batchItem.ID
	}
	return download
}

const (
	audíoAutoDisp    = "-disposition:a auto"
	SubtitleAutoDisp = "-disposition:s auto"
)

func formatArgs(args, title, episode, season, metaName, dlPath string) string {
	if title != "" {
		if strings.Contains(args, "This Episode") {
			info := title
			if episode != "" {
				info = "Episode " + episode + " of " + info
			}
			if season != "" {
				info += " Season " + season
			}
			args = strings.ReplaceAll(args, "This Episode", utils.ShellSafeText(info))
		}
	}
	if strings.Contains(args, "Fileinfo") {
		args = strings.ReplaceAll(args, "Fileinfo", utils.ShellSafeText(metaName))
	}
	if strings.Contains(args, audíoAutoDisp) || strings.Contains(args, SubtitleAutoDisp) {
		aPosInSteam, sPosInSteam := utils.PosInStream(dlPath, "eng", "eng-us", "")
		if strings.Contains(args, audíoAutoDisp) {
			if aPosInSteam != -1 {
				args = strings.ReplaceAll(
					args,
					audíoAutoDisp,
					"-disposition:a 0 -disposition:a:"+strconv.Itoa(aPosInSteam)+" default",
				)
			} else {
				args = strings.ReplaceAll(args, audíoAutoDisp, "-disposition:a 0")
			}
		}
		if strings.Contains(args, SubtitleAutoDisp) {
			if sPosInSteam != -1 {
				args = strings.ReplaceAll(
					args,
					SubtitleAutoDisp,
					"-disposition:s 0 -disposition:s:"+strconv.Itoa(sPosInSteam)+" default",
				)
			} else {
				args = strings.ReplaceAll(args, SubtitleAutoDisp, "-disposition:s 0")
			}
		}
	}
	return args
}

func sendBanner(fchannel int64, fmsgID int, fbanner bool) *tele.Message {
	if fchannel == 0 || config.Conf.NoBanner || !fbanner {
		return nil
	}
	sendOpts := &tele.SendOptions{}
	if fmsgID != 0 {
		ID := constant.TDLibPeerID(fchannel)
		sendOpts.ReplyMarkup = markup.InlineKeyboard(
			markup.Row(
				markup.URL(
					"Status",
					fmt.Sprintf("https://t.me/c/%d/%d", ID.ToPlain(), fmsgID),
				),
			),
		)
	}
	Bmsg, _ := tele.SendMsg(fchannel, "**New Release!**", sendOpts)
	return Bmsg
}

func NextBatchItem(items []types.BatchItem, except string) *types.BatchItem {
	tempItems := slices.Clone(items)
	for i := range tempItems {
		if tempItems[i].ID == except {
			tempItems = append(tempItems[:i], tempItems[i+1:]...)
			break
		}
	}
	return batchutils.GetNextBatchItem(tempItems)
}

func sendSticker(entry types.QueueEntry, batchItem *types.BatchItem) *tele.Message {
	sticker := config.Conf.FSticker
	if sticker == "" {
		return nil
	}
	if len(entry.Batch) > 0 {
		next := NextBatchItem(entry.Batch, batchItem.ID)
		if next != nil {
			c, _ := parser.QParseT(batchItem.FileName, entry.Version, entry.Filter)
			n, _ := parser.QParseT(next.FileName, entry.Version, entry.Filter)
			if c == n {
				return nil
			}
		}
	} else if entries := bot.Queue.AsSlice(); len(entries) > 1 {
		i := 1
		if entry.Name != entries[0].Name {
			i = 0
		}
		c, _ := parser.QParseT(entry.Name, entry.Version, entry.Filter)
		n, _ := parser.QParseT(entries[i].Name, entries[i].Version, entries[i].Filter)
		if c == n {
			return nil
		}
	}
	msg, err := tele.SendStickerByFileID(
		config.Conf.FChannel,
		sticker,
	)
	if err != nil {
		logger.Error(err)
	}
	return msg
}

func cacheNextDownload(entries []types.QueueEntry, batchItem *types.BatchItem) {
	if len(entries) == 0 {
		return
	}
	cacheBatchItem := func(entry types.QueueEntry, batchItem *types.BatchItem) {
		download := &downloaders.Downloader{
			Folder:   config.Conf.DownloadDir,
			FileName: entry.Name,
			Selected: batchItem.ID,
			DlInfo:   true,
			URI:      entry.URI,
		}
		switch entry.Source.Source {
		case source.Qbit:
			return
		case source.Ytdl:
			download.Ytdl = true
		default:
			if !utils.IsMegaLink(entry.URI) {
				return
			}
		}
		download.Go()
		downloaders.CachedDL[entry.Name+"++"+batchItem.ID] = download
	}
	if batchItem != nil && len(entries[0].Batch) != 0 {
		next := NextBatchItem(entries[0].Batch, batchItem.ID)
		if next != nil {
			cacheBatchItem(entries[0], next)
			return
		}
	}
	if len(entries) < 2 {
		return
	}
	entry := entries[1]
	if len(entry.Batch) != 0 {
		next := batchutils.GetNextBatchItem(entry.Batch)
		if next != nil {
			cacheBatchItem(entry, next)
		}
		return
	}
	download := &downloaders.Downloader{
		DlInfo:   true,
		Folder:   config.Conf.DownloadDir,
		Selected: entry.Source.Select,
		URI:      entry.URI,
		FileName: entry.Name,
	}
	switch entry.Source.Source {
	case source.Telegram:
		if entry.Doc == nil {
			// this should never ordinarily happen but just in case
			logger.WarnW(
				"Queued item from telegram is missing file meta",
				zap.String("Name", entry.Name),
			)
			return
		}
		loc, err := utils.BuildInputFileLocation(entry.Doc)
		if err != nil {
			logger.ErrorW(
				"Could not retrieve input file location from queued item",
				zap.Error(err),
			)
			return
		}
		download.InputLoc = loc
		download.Size = entry.Doc.Size
	case source.Qbit:
		if download.Selected != "" {
			return
		}
		download.Qbit = true
	case source.Ytdl:
		download.Ytdl = true
	case source.Aria2:
		if download.Selected != "" {
			return
		}
	}
	download.Go()
	downloaders.CachedDL[entry.Name] = download
}
