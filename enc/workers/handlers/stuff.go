package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/tele"
)

func StartEx(ctx *ext.Context, update *ext.Update) error {
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
	fmt.Println(update.EffectiveMessage)
	_, _ = ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		Alert:   true,
		QueryID: query.QueryID,
		Message: "This is an example bot!",
	})
	return nil
}

// UpHandler implements the ping/pong behaviour in canonical signature form.
func Up(ctx *ext.Context, update *ext.Update, msg *tele.Message) error {
	if !utils.UserIsAllowed(msg.SenderID) {
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

func Start(ctx *ext.Context, update *ext.Update, msg *tele.Message) error {
	user := update.EffectiveUser()
	chat := update.EffectiveChat()
	currentTime := utils.TimeFormatter(time.Since(bot.BotStartTime).Round(time.Second).Seconds()) // TF matches your tf helper

	// base messages
	msg1 := fmt.Sprintf("Hi `%s`\n", user.FirstName)
	msg2 := fmt.Sprintf("%sI've been alive for `%s` and i'm ready to encode videos 😗", msg1, currentTime)
	msg3 := fmt.Sprintf("%s\nand by the way you're a temporary user", msg2)

	var outMsg string

	// If the user isn't owner and this is a private chat, check PM permission
	if !utils.UserIsOwner(msg.SenderID) && chat.IsAUser() {
		if !utils.AllowPM() {
			return nil
		}
	}

	// Choose message depending on temporary/allowed/user status
	if utils.UserIsTemp(msg.SenderID) {
		outMsg = msg3
	} else if !utils.UserIsAllowed(msg.SenderID) {
		// build not-allowed message with a link to owner
		ownerIDStr := strings.Split(config.Conf.Owner, " ")[0]
		ownerID, _ := strconv.ParseInt(
			ownerIDStr,
			10,
			64,
		)
		user, err := tele.GetUser(ownerID)
		ownerName := ownerIDStr
		if err == nil && user != nil {
			ownerName = user.FirstName
		}
		outMsg = fmt.Sprintf("%sYou're not allowed access to this bot\nAsk [%s](tg://user?id=%s) (nicely) to grant you access.", msg1, ownerName, ownerIDStr)
	} else {
		outMsg = msg2
	}
	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{
			{
				Buttons: []tg.KeyboardButtonClass{
					&tg.KeyboardButtonCallback{
						Text: "Help",
						Data: []byte("ihelp"),
					},
				},
			},
			{
				Buttons: []tg.KeyboardButtonClass{
					&tg.KeyboardButtonURL{
						Text: "Source Code",
						URL:  "https://github.com/Nubuki-all/Enc-Go",
					},
					&tg.KeyboardButtonURL{
						Text: "Developer",
						URL:  "https://t.me/Col_serra",
					},
				},
			},
		},
	}

	// Buttons (use your project's Button helpers).
	// I assume utils.ButtonInline and utils.ButtonURL exist and ReplyWithButtons sends the message + buttons.

	// Send reply with buttons using your helper (reply/edit wrapper)
	_, err := msg.Reply(
		outMsg,
		&tele.ReplyOptions{
			ReplyMarkup: markup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return err
	}
	return nil
}
