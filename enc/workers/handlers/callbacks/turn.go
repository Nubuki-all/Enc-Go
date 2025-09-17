package callbacks

import (
	"fmt"
	"strings"

	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/telegram/message/markup"
	//"github.com/gotd/td/tg"

	"enc/internal/bot"
	//"enc/internal/logger"
	"enc/utils"
	"enc/utils/tele"
)

func CancelRTurn(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
    //id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
    _, turnID, _ := strings.Cut(string(query.Query.Data), " ")
    if !bot.WaitQueueR.Contains(turnID) {
        return cleanOldMessage(ctx, update, query)
    }
    sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	_ = query.Answer("Cancelling…", false)
	_ = bot.WaitQueueR.Remove(turnID)
	_ = query.GetMessage().Delete()
	return nil
}