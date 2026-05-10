package manage

import (
	"os"

	"github.com/celestix/gotgproto/ext"

	"enc/internal/bot"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func Nuke(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}

	_, _ = msg.ReplyText("`Nuking the bot. Goodbye…`")

	downloaders.CleanAllQbit(ctx)
	db.SaveAll(ctx)
	if bot.Aria2Client != nil {
		bot.Aria2Client.Close() //nolint:errcheck
	}

	os.Exit(0)
	return nil
}
