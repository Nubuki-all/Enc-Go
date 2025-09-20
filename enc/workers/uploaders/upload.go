// Package uploaders handles uploading of files
package uploaders

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"

	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

const (
	CallbackData = "cancel_upload"
)

type Uploader struct {
	*types.TransferInfo
	Msg             *tele.Message
	Reply           *tele.Message
	Caption         string
	FileName        string
	ThumbPath       string
	Bytes           []byte
	CleanOnComplete bool
	Done            bool
	ForceDocument   bool
	err             error
}

func (ul *Uploader) Init(id, path string, sender int64) {
	if ul.TransferInfo == nil {
		ul.TransferInfo = &types.TransferInfo{
			ID:     id,
			Sender: sender,
			Time:   time.Now(),
			Path:   path,
		}
	}
	ul.Register()
	ul.Msg, _ = ul.Msg.Edit(
		"<code>Waiting for upload to start…</code>",
		&tele.EditOptions{
			HtmlParseMode: true,
			ReplyMarkup:   ul.GenButtons(),
		},
	)
}

func (ul *Uploader) Start() error {
	defer ul.UnRegister()
	var err error
	if ul.Path != "" && ul.FileName == "" {
		ul.FileName = filepath.Base(ul.Path)
	}
	tgUl := NewTgUl(ul.FileName, ul.Caption, ul.ThumbPath, ul.ForceDocument, ul.Msg, ul.Reply, ul.TransferInfo)
	err = tgUl.Start()
	if err != nil {
		ul.err = err
		logger.Error(err)
	} else if ul.CleanOnComplete {
		ul.CleanUpload()
	}
	ul.Done = true
	return err
}

func (ul *Uploader) CleanUpload() {
	err := os.Remove(ul.Path)
	if err != nil {
		logger.Error(err)
	}
}

func (ul *Uploader) GenButtons() tg.ReplyMarkupClass {
	buttonRows := []tg.KeyboardButtonRow{
		markup.Row(
			markup.Callback(
				random.Enmoji()+" Cancel Upload",
				[]byte(CallbackData),
			),
		),
	}
	return markup.InlineKeyboard(buttonRows...)
}

func (ul *Uploader) Register() {
	callbackInfo := &types.CallbackInfo{
		TransferInfo: ul.TransferInfo,
	}
	_ = utils.StoreCallbackInfo(callbackInfo, ul.ID)
}

func (ul *Uploader) UnRegister() {
	utils.DelCallbackInfo(ul.ID)
}

func (ul *Uploader) ReportStatus() (*tele.Message, error) {
	var status string
	var err error
	if ul.IsCancelled {
		status = "was cancelled"
		if cncl := ul.Canceller; cncl != 0 && cncl != ul.Sender {
			userName := "Unknown"
			user, err := tele.GetUser(cncl)
			if err == nil {
				userName = user.FirstName
			}
			status = fmt.Sprintf("%s by [%s](tg://user?id=%d)", status, userName, cncl)
		}
		status += "."
	} else if err = ul.err; err == nil {
		status = "was completed successfully."
	} else {
		status = fmt.Sprintf("failed with error:\n`%s`", err.Error())
	}
	text := fmt.Sprintf("Upload of `%s` %s", ul.FileName, status)

	return ul.Msg.Edit(text,
		&tele.EditOptions{},
	)
}
