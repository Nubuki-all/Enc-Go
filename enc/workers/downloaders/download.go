// Package downloaders downloads files from different sources.
package downloaders

import (
	"fmt"
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
	CallbackDataM = "more"
)

type Downloader struct {
	*types.TransferInfo
	LCMsg         *tele.Message
	Msg           *tele.Message
	InputLoc      tg.InputFileLocationClass
	service       dlService
	DlInfo        bool
	Done          chan struct{}
	Folder        string
	FileName      string
	Selected      string
	URI           string
	Size          int64
	Qbit          bool
	Ytdl          bool
	Background    bool
	CompletedTime float64
	err           error
	cleaned       bool
	pauseOnDLInfo bool
}

type dlService interface {
	AttachMsg(msg *tele.Message)
	GetFileName() string
	Start() error
	Clean()
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
	dl.initLMsg()
}

func (dl *Downloader) Go() {
	if dl.TransferInfo == nil {
		dl.TransferInfo = &types.TransferInfo{
			Time: time.Now(),
		}
	}
	dl.pauseOnDLInfo = config.Conf.PauseOnDLInfo
	dl.Background = true
	dl.PauseProgress = true
	go func() { _ = dl.Start() }()
}

func (dl *Downloader) Attach(id string, Sender int64) error {
	if !dl.Background {
		logger.Warn("Calling Attach on Non-background download!")
		return nil
	}
	select {
	case <-dl.Done:
		return dl.err
	default:
	}
	dl.Background = false
	dl.Msg, _ = dl.Msg.Edit(
		"<code>Waiting for download handler…</code>",
		&tele.EditOptions{
			HTMLParseMode: true,
			ReplyMarkup:   dl.GenButtons(),
		},
	)
	dl.service.AttachMsg(dl.Msg)
	dl.ID = id
	dl.Sender = Sender
	dl.Register()
	defer dl.UnRegister() // Just in case
	dl.PauseProgress = false
	dl.initLMsg()
	<-dl.Done
	return dl.err
}

func (dl *Downloader) Start() error {
	if dl.Folder == "" {
		dl.Folder = config.Conf.DownloadDir
	}
	defer dl.UnRegister()
	dl.Done = make(chan struct{})
	defer func() {
		close(dl.Done)
	}()
	dl.Path = filepath.Join(dl.Folder, dl.FileName)
	folder, _ := filepath.Abs(dl.Folder)
	if dl.InputLoc != nil && !utils.IsInsideDir(dl.Folder, dl.Path) {
		dl.err = fmt.Errorf("refusing to download %q outside %q", dl.FileName, dl.Folder)
		return dl.err
	}
	if dl.InputLoc != nil {
		dl.service = NewTgDl(dl.InputLoc, dl.Size, dl.Msg, dl.TransferInfo)
	} else if dl.Qbit && dl.URI != "" {
		dl.service = NewQbtitDl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
	} else if dl.Ytdl && dl.URI != "" {
		dl.service = NewYTDl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
	} else if dl.URI != "" && utils.IsMegaLink(dl.URI) {
		dl.service = NewMegaDl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
	} else if dl.URI != "" {
		dl.service = NewAria2Dl(dl.URI, folder, dl.Selected, dl.Msg, dl.TransferInfo)
	}
	err := dl.service.Start()
	if err == nil && dl.Background {
		dl.CompletedTime = time.Since(dl.Time).Round(time.Second).Seconds()
	}
	if name := dl.service.GetFileName(); name != "" {
		dl.FileName = name
	}
	if err != nil || dl.IsCancelled { // for downloaders that don't return an err on download cancel
		dl.err = err
		dl.CleanDownload()
		if !dl.IsCancelled {
			logger.Error(err)
		}
	}
	return err
}

func (dl *Downloader) CleanDownload() {
	if dl.cleaned {
		return
	}
	dl.cleaned = true
	if dl.service != nil {
		dl.service.Clean()
	}
}

func (dl *Downloader) GenButtons() tg.ReplyMarkupClass {
	buttonRows := []tg.KeyboardButtonRow{}
	if dl.DlInfo {
		buttonRows = []tg.KeyboardButtonRow{
			markup.Row(
				markup.Callback(
					"ℹ️",
					[]byte(CallbackDataI),
				),
			),
		}
	}
	buttonRows = append(
		buttonRows,
		markup.Row(
			markup.Callback(
				random.Enmoji()+" Cancel Download",
				[]byte(CallbackData),
			),
		),
	)
	return markup.InlineKeyboard(buttonRows...)
}

func (dl *Downloader) initLMsg() {
	if dl.LCMsg == nil {
		return
	}
	_, _ = dl.LCMsg.Edit(
		"Currently downloading an item",
		&tele.EditOptions{ReplyMarkup: dl.genLButtons()},
	)
}

func (dl *Downloader) genLButtons() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.Row(
			markup.Callback(
				"ℹ️",
				[]byte(CallbackDataM),
			),
		),
		markup.Row(
			markup.Callback(
				random.Enmoji()+" CANCEL DOWNLOAD",
				[]byte(CallbackData),
			),
		),
	)
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
			status = fmt.Sprintf("%s by <a href=\"tg://user?id=%d\">%s</a>", status, cncl, userName)
		}
		status += "."
	} else if err = dl.err; err == nil {
		status = "was completed successfully."
	} else {
		status = fmt.Sprintf("failed with error:\n<code>%s</code>", err.Error())
	}
	text := fmt.Sprintf("Download of <code>%s</code> %s", dl.FileName, status)
	msg := dl.Msg

	if bot.ReportFailedDl {
		reportee := config.Rconf.Owners[0]
		if reportee != msg.ChatID && dl.err != nil && !dl.IsCancelled {
			go func(text string, id int64) {
				_, err := tele.SendMsg(id, text, &tele.SendOptions{HTMLParseMode: true})
				if err != nil {
					logger.Error(err)
				}
			}(text, reportee)
		}
	}
	return msg.Edit(
		text,
		&tele.EditOptions{HTMLParseMode: true},
	)
}
