// Package uploaders handles uploading of files
package uploaders

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"

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
	TgMedia         tg.InputMediaClass // contains uploaded media
	Caption         string
	FileName        string
	ThumbPath       string
	Threads         int
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
			HTMLParseMode: true,
			ReplyMarkup:   ul.GenButtons(),
		},
	)
}

func (ul *Uploader) Start() error {
	defer ul.UnRegister()

	if ul.Path != "" && ul.FileName == "" {
		ul.FileName = filepath.Base(ul.Path)
	}

	defer func() { ul.Done = true }()
	maxAttempts := 3
	for i := range maxAttempts {
		tgUl := NewTgUl(ul.FileName, ul.Caption, ul.ThumbPath, ul.ForceDocument, ul.Msg, ul.Reply, ul.TransferInfo)
		if ul.Threads > 0 {
			tgUl.threads = ul.Threads
		}
		err := tgUl.Start()

		if err == nil {
			ul.TgMedia = tgUl.media
			if ul.CleanOnComplete {
				ul.CleanUpload()
			}
			return nil
		}

		if errors.Is(err, ErrRetry) && i < maxAttempts-1 {
			logger.WarnW(
				"Upload failed due to connection, retrying after wait...",
				zap.Int("attempt", i+1),
				zap.Error(err),
			)
			continue
		}

		ul.err = err
		logger.Error(err)
		return err
	}
	return fmt.Errorf("upload failed after maximum retries")
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
	if ul.IsCancelled.Load() {
		status = "was cancelled"
		if cncl := ul.Canceller; cncl != 0 && cncl != ul.Sender {
			userName := "Unknown"
			user, err := tele.GetUser(cncl)
			if err == nil {
				userName = user.FirstName
			}
			status = fmt.Sprintf("%s by <a href=\"tg://user?id=%d\">%s</a>", status, cncl, userName)
		}
		status += "."
	} else if err = ul.err; err == nil {
		status = "was completed successfully."
	} else {
		status = fmt.Sprintf("failed with error:\n<code>%s</code>", err.Error())
	}
	text := fmt.Sprintf("Upload of <code>%s</code> %s", ul.FileName, status)

	return ul.Msg.Edit(
		text,
		&tele.EditOptions{HTMLParseMode: true},
	)
}
