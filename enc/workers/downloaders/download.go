package downloaders

import (
	"os"
	"time"

	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

const (
	callbackData  = "cancel_download"
	callbackDataB = "back"
	callbackDataI = "dl_info"
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
	if err != nil {
		logger.Error(err)
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
				[]byte(callbackData),
			),
		),
	}
	if dl.DlInfo {
		buttonRows = append(buttonRows,
			markup.Row(
				markup.Callback(
					"ℹ️",
					[]byte(callbackDataI),
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
