package manage

import (
	"context"
	"os"


	"enc/internal/bot"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func Nuke(msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}

	_, _ = msg.ReplyText("`Nuking the bot. Goodbye…`")

	downloaders.CleanAllQbit(ctx)
	db.SaveAll(context.Background())
	if bot.Aria2Client != nil {
		bot.Aria2Client.Close() //nolint:errcheck
	}

	os.Exit(0)
	return nil
}
