package manage

import (
	"os"

	"enc/internal/bot"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func Nuke(ctx *tele.Context, update *tele.Update, msg *tele.Message, args string) error {
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
