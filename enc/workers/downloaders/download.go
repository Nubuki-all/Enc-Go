package downloaders

import (
	"fmt"
	"os"
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
	DlInfo        bool
	Folder        string
	FileName      string
	Uri           string
	Size          int64
	Qbit          bool
	err           error
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
			HtmlParseMode: true,
			ReplyMarkup:   dl.GenButtons(),
		},
	)
}

func (dl *Downloader) Start() error {
	dl.Path = dl.Folder + dl.FileName
	var err error
	if dl.InputLoc != nil {
		tgDl := NewTgDl(dl.InputLoc, dl.Size, dl.Msg, dl.TransferInfo)
		err = tgDl.Start()
	}
	dl.UnRegister()
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
	err := os.Remove(dl.Path)
	logger.Error(err)
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
	if err := dl.err; err == nil {
		status = "was completed successfully."
	} else if dl.IsCancelled {
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
	} else {
		status = fmt.Sprintf("failed with error:\n`%s`", dl.err.Error())
	}
	text := fmt.Sprintf("Download of `%s` %s", dl.FileName, status)
	msg := dl.Msg
	
	if bot.ReportFailedDl {
		reportee := config.Rconf.Owners[0]
		if reportee != msg.ChatID && dl.err != nil && !dl.IsCancelled {
			go func (text string, id int64) {
				err:= tele.SendMessage(id, text)
				if err != nil {
					logger.Error()
				}
			}(text, reportee)
		}
	}
	return msg.Edit(text,
		&tele.EditOptions{},
	)
}
