package callbacks

import (
	"fmt"
	"path/filepath"
	//"strings"

	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/telegram/message/markup"
	//"github.com/gotd/td/tg"

	//"enc/internal/bot"
	//"enc/internal/logger"
	"enc/utils"
	"enc/utils/tele"
)

const notAllowed = "You're not allowed to do this!"

func CancelDownload(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	_ = query.Answer("Cancelling download please wait…", false)
	info.IsCancelled = true
	info.Canceller = sender
	return nil
}

func CancelUpload(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	_ = query.Answer("Cancelling upload please wait…", false)
	info.IsCancelled = true
	info.Canceller = sender
	return nil
}

func EncodeInfo(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	if info.ExtraFlags == "mux" {
		return query.Answer("Muxing:\n"+filepath.Base(info.OutPath), true)
	}
	return nil
}
