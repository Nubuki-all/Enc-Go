package manage

import (
	"github.com/celestix/gotgproto/ext"

	"enc/internal/bot"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func Restart(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Restarts bot."
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}
	rMsg, err := msg.ReplyText("`Restarting Please Wait…`")
	if err != nil {
		return nil
	}
	downloaders.CleanAllQbit(ctx)
	db.SaveAll(ctx)
	if bot.Aria2Client != nil {
		bot.Aria2Client.Close() //nolint:errcheck
	}
	// err = utils.ExecRestart(msg.ChatID, rMsg.ID)
	err = utils.Restart(msg.ChatID, rMsg.ID)
	if err != nil {
		_, _ = rMsg.EditText("**Restart failed:** " + err.Error())
	}
	return nil
}
