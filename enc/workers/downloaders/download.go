// Package downloaders downloads files from different sources.
package downloaders

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

const (
	CallbackData  = "cancel_download"
	CallbackDataB = "back"
	CallbackDataI = "dl_info"
)

type Downloader struct {
	*types.TransferInfo
	LCMsg         *tele.Message
	Msg           *tele.Message
	InputLoc      tg.InputFileLocationClass
	aria2Dl       *Aria2Download
	qbitDl        *QbitDownload
	megaDl        *MegaDownload
	ytDl          *YtdlDownload
	DlInfo        bool
	Done          bool
	Folder        string
	FileName      string
	Selected      string
	URI           string
	Size          int64
	Qbit          bool
	Ytdl          bool
	err           error
	cleaned       bool
	pauseOnDLInfo bool
}

func (dl *Downloader) Init(id string, sender int64) {
	if dl.TransferInfo == nil {
		dl.TransferInfo = &types.TransferInfo{
			ID:     id,
			Sender: sender,
			Time:   time.Now(),
		}
	}
	dl.pauseOnDLInfo = config.Conf.PauseOnDLInfo
	dl.Register()
	dl.Msg, _ = dl.Msg.Edit(
		"<code>Waiting for download to start…</code>",
		&tele.EditOptions{
			HTMLParseMode: true,
			ReplyMarkup:   dl.GenButtons(),
		},
	)
}

func (dl *Downloader) Start() error {
	if dl.Folder == "" {
		dl.Folder = config.Conf.DownloadDir
	}
	defer dl.UnRegister()
	dl.Path = filepath.Join(dl.Folder, dl.FileName)
	var err error
	if dl.InputLoc != nil {
		tgDl := NewTgDl(dl.InputLoc, dl.Size, dl.Msg, dl.TransferInfo)
		err = tgDl.Start()
	} else if dl.Qbit && dl.URI != "" {
		folder, _ := filepath.Abs(dl.Folder)
		dl.qbitDl = NewQbtitDl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
		err = dl.qbitDl.Start()
		if err == nil {
			dl.FileName = dl.qbitDl.name
		}
	} else if dl.Ytdl && dl.URI != "" {
		folder, _ := filepath.Abs(dl.Folder)
		dl.ytDl = NewYTDl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
		err = dl.ytDl.Start()
		if err == nil {
			dl.FileName = dl.ytDl.name
		}

	} else if dl.URI != "" && utils.IsMegaLink(dl.URI) {
		folder, _ := filepath.Abs(dl.Folder)
		dl.megaDl = NewMegaDl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
		err = dl.megaDl.Start()
		if err == nil {
			dl.FileName = filepath.Base(dl.megaDl.Path)
		}
	} else if dl.URI != "" {
		folder, _ := filepath.Abs(dl.Folder)
		dl.aria2Dl = NewAria2Dl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
		err = dl.aria2Dl.Start()
		if err == nil {
			dl.FileName = dl.aria2Dl.name
		}
	}
	if err != nil || dl.IsCancelled { // for downloaders that don't return an err on download cancel
		dl.err = err
		dl.CleanDownload()
		if !dl.IsCancelled {
			logger.Error(err)
		}
	}
	dl.Done = true
	return err
}

func (dl *Downloader) CleanDownload() {
	if dl.cleaned {
		return
	}
	dl.cleaned = true
	if dl.aria2Dl != nil {
		dl.aria2Dl.Clean()
		return
	}
	if dl.qbitDl != nil {
		dl.qbitDl.Clean()
		return
	}
	if dl.megaDl != nil {
		dl.megaDl.Clean()
		return
	}
	if dl.ytDl != nil {
		dl.ytDl.Clean()
		return
	}
	err := os.Remove(dl.Path)
	if err != nil {
		logger.Error(err)
	}
}

func (dl *Downloader) GenButtons() tg.ReplyMarkupClass {
	buttonRows := []tg.KeyboardButtonRow{
		markup.Row(
			markup.Callback(
				random.Enmoji()+" Cancel Download",
				[]byte(CallbackData),
			),
		),
	}
	if dl.DlInfo {
		buttonRows = append(buttonRows,
			markup.Row(
				markup.Callback(
					"ℹ️",
					[]byte(CallbackDataI),
				),
			),
		)
	}
	return markup.InlineKeyboard(buttonRows...)
}

func (dl *Downloader) Register() {
	callbackInfo := &types.CallbackInfo{
		TransferInfo: dl.TransferInfo,
	}
	_ = utils.StoreCallbackInfo(callbackInfo, dl.ID)
}

func (dl *Downloader) UnRegister() {
	utils.DelCallbackInfo(dl.ID)
}

func (dl *Downloader) ReportStatus() (*tele.Message, error) {
	var status string
	var err error
	if dl.IsCancelled {
		status = "was cancelled"
		if cncl := dl.Canceller; cncl != 0 && cncl != dl.Sender {
			userName := "Unknown"
			user, err := tele.GetUser(cncl)
			if err == nil {
				userName = user.FirstName
			}
			status = fmt.Sprintf("%s by [%s](tg://user?id=%d)", status, userName, cncl)
		}
		status += "."
	} else if err = dl.err; err == nil {
		status = "was completed successfully."
	} else {
		status = fmt.Sprintf("failed with error:\n`%s`", err.Error())
	}
	text := fmt.Sprintf("Download of `%s` %s", dl.FileName, status)
	msg := dl.Msg

	if bot.ReportFailedDl {
		reportee := config.Rconf.Owners[0]
		if reportee != msg.ChatID && dl.err != nil && !dl.IsCancelled {
			go func(text string, id int64) {
				err := tele.SendMessage(id, text)
				if err != nil {
					logger.Error(err)
				}
			}(text, reportee)
		}
	}
	return msg.Edit(text,
		&tele.EditOptions{},
	)
}
