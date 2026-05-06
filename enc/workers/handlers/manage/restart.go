package manage

import (
	"github.com/celestix/gotgproto/ext"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func Restart(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Restarts bot.\n" +
		"  -d, --detach: To restart in detached mode.\n" +
		"  -nd, --no-detach: To restart in non-detached mode."

	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	arg := argparser.ParseFlags(
		"restart",
		helpMsg,
		argList,
		argparser.CreateDBoolFlag("detach", "d", "To restart in detached mode."),
		argparser.CreateDBoolFlag("no-detach", "nd", "To restart in non-detached mode."),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(
			utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}

	pargs := arg.Values
	detach := pargs["detach"].(bool)
	noDetach := pargs["no-detach"].(bool)

	isDetached := config.Rconf.Detached
	if detach {
		isDetached = true
	} else if noDetach {
		isDetached = false
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
	err = utils.Restart(msg.ChatID, rMsg.ID, isDetached)
	if err != nil {
		_, _ = rMsg.EditText("**Restart failed:** " + err.Error())
	}
	return nil
}
