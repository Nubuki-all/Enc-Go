package callbacks

import (
	"fmt"
	//"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/telegram/message/markup"
	//"github.com/gotd/td/tg"

	//"enc/internal/bot"
	//"enc/internal/logger"
	"enc/utils"
	"enc/utils/tele"
)


func cleanOldMessage(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
    _ = query.Answer("⛔ Old button.  Removing…", false)
    time.Sleep(5 * time.Second)
    err := query.GetMessage().Delete()
	return err
}