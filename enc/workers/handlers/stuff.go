package handlers

import (
	"fmt"
	"time"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
	
	"enc/utils"
	"enc/utils/tele"
)

func Start(ctx *ext.Context, update *ext.Update) error {
	user := update.EffectiveUser()
	_, _ = ctx.Reply(update, ext.ReplyTextString(fmt.Sprintf("Hello %s, I am @%s and will repeat all your messages.\nI was made using gotd and gotgproto.", user.FirstName, ctx.Self.Username)), &ext.ReplyOpts{
		Markup: &tg.ReplyInlineMarkup{
			Rows: []tg.KeyboardButtonRow{
				{
					Buttons: []tg.KeyboardButtonClass{
						&tg.KeyboardButtonURL{
							Text: "gotd/td",
							URL:  "https://github.com/gotd/td",
						},
						&tg.KeyboardButtonURL{
							Text: "gotgproto",
							URL:  "https://github.com/celestix/gotgproto",
						},
					},
				},
				{
					Buttons: []tg.KeyboardButtonClass{
						&tg.KeyboardButtonCallback{
							Text: "Click Here",
							Data: []byte("cb_pressed"),
						},
					},
				},
			},
		},
	})
	// End dispatcher groups so that bot doesn't echo /start command usage
	return dispatcher.EndGroups
}

func ButtonCallback(ctx *ext.Context, update *ext.Update) error {
	query := update.CallbackQuery
	_, _ = ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		Alert:   true,
		QueryID: query.QueryID,
		Message: "This is an example bot!",
	})
	return nil
}


// UpHandler implements the ping/pong behaviour in canonical signature form.
func Up(ctx *ext.Context, update *ext.Update, msg *tele.Message) error {
	if !utils.UserIsAllowed(msg.userID) {
		// You chose earlier to delete incoming message when not allowed — implement in utils if needed
		return nil
	}

	// send placeholder
	start := time.Now()
	repMsg, err := msg.ReplyText("…")
	if err != nil {
		return err
	}
	sendLatency := time.Since(start).Milliseconds()

	// edit first timing
	msgFmt := "**Pong! ——** `%d`__ms__"
	if _, err := repMsg.EditText(fmt.Sprintf(msgFmt, sendLatency)); err != nil {
		return err
	}

	// measure edit latency (difference between now and send start)
	editLatency := time.Since(start).Milliseconds() - sendLatency
	final := fmt.Sprintf("1. %s\n2. %s", fmt.Sprintf(msgFmt, sendLatency), fmt.Sprintf(msgFmt, editLatency))
	if _, err := repMsg.EditText(final); err != nil {
		return err
	}
	return nil
}
