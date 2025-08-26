package handlers

import (
	//"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/tele"
)

func Up(ctx *ext.Context, update *ext.Update, msg *tele.Message) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		// To delete()
		return nil
	}
	start := time.Now()
	repMsg, err := msg.ReplyText("…")
	if err != nil {
		return err
	}
	sendLatency := time.Since(start).Milliseconds()
	msgFmt := "**Pong! ——** `%d`__ms__"
	if _, err := repMsg.EditText(fmt.Sprintf(msgFmt, sendLatency)); err != nil {
		return err
	}
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
	currentTime := utils.TimeFormatter(time.Since(bot.BotStartTime).Round(time.Second).Seconds())
	msg1 := fmt.Sprintf("Hi `%s`\n", user.FirstName)
	msg2 := fmt.Sprintf("%sI've been alive for `%s` and i'm ready to encode videos 😗", msg1, currentTime)
	msg3 := fmt.Sprintf("%s\nand by the way you're a temporary user", msg2)

	var outMsg string
	if !utils.UserIsOwner(msg.SenderID) && chat.IsAUser() {
		if !utils.AllowPM() {
			return nil
		}
	}
	if utils.UserIsTemp(msg.SenderID) {
		outMsg = msg3
	} else if !utils.UserIsAllowed(msg.SenderID) {
		ownerID := config.Rconf.Owners[0]
		user, err := tele.GetUser(ownerID)
		ownerName := strconv.FormatInt(ownerID, 10)
		if err == nil && user != nil {
			ownerName = user.FirstName
		}
		outMsg = fmt.Sprintf("%sYou're not allowed access to this bot\nAsk [%s](tg://user?id=%s) (nicely) to grant you access.", msg1, ownerName, ownerIDStr)
	} else {
		outMsg = msg2
	}
	replyMarkup := &tg.ReplyInlineMarkup{
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
	_, err := msg.Reply(
		outMsg,
		&tele.ReplyOptions{
			ReplyMarkup: replyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return err
	}
	return nil
}

func Help(ctx *ext.Context, update *ext.Update, msg *tele.Message) error {
	chat := update.EffectiveChat()
	if !utils.UserIsOwner(msg.SenderID) && chat.IsAUser() {
		if !utils.AllowPM() {
			query := update.CallbackQuery
			_, _ = ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
				Alert:   false,
				QueryID: query.QueryID,
				Message: "Nope!",
			})
			return nil
		}
	}
	text := "**⛩️ An Encode bot**\n\n+" +
		"This bot encodes videos With your custom ffmpeg or handbrake-cli settings." +
		"\n+Easy to Use (Depends)\n" +
		"-Due to your custom Settings & hosting server bot may or may not take a long time to encode." +
		"\n\nFor available commands click the Commands button below."
	replyMarkup := markup.InlineKeyboard(
		markup.Row(
			markup.Callback(
				"Commands",
				[]byte("icommands"),
			),
		),
		markup.Row(
			markup.Callback(
				"🔙 Back",
				[]byte("back_cb"),
			),
		),
	)
	_, err := msg.Edit(
		text,
		&tele.EditOptions{
			ReplyMarkup: replyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return err
	}
	return nil
}

func StartBack(ctx *ext.Context, update *ext.Update, msg *tele.Message) error {
	user := update.EffectiveUser()
	chat := update.EffectiveChat()
	currentTime := utils.TimeFormatter(time.Since(bot.BotStartTime).Round(time.Second).Seconds())
	msg1 := fmt.Sprintf("Hi `%s`\n", user.FirstName)
	msg2 := fmt.Sprintf("%sI've been alive for `%s` and i'm ready to encode videos 😗", msg1, currentTime)
	msg3 := fmt.Sprintf("%s\nand by the way you're a temporary user", msg2)

	var outMsg string
	if !utils.UserIsOwner(msg.SenderID) && chat.IsAUser() {
		if !utils.AllowPM() {
			return nil
		}
	}
	if utils.UserIsTemp(msg.SenderID) {
		outMsg = msg3
	} else if !utils.UserIsAllowed(msg.SenderID) {
		ownerID := config.Rconf.Owners[0]
		user, err := tele.GetUser(ownerID)
		ownerName := strconv.FormatInt(ownerID, 10)
		if err == nil && user != nil {
			ownerName = user.FirstName
		}
		outMsg = fmt.Sprintf("%sYou're not allowed access to this bot\nAsk [%s](tg://user?id=%s) (nicely) to grant you access.", msg1, ownerName, ownerIDStr)
	} else {
		outMsg = msg2
	}

	replyMarkup := markup.InlineKeyboard(
		markup.Row(
			markup.Callback(
				"Help",
				[]byte("ihelp"),
			),
		),
		markup.Row(
			markup.URL(
				"Source Code",
				"https://github.com/Nubuki-all/Enc-Go",
			),
			markup.URL(
				"Developer",
				"https://t.me/Col_serra",
			),
		),
	)
	_, err := msg.Edit(
		outMsg,
		&tele.EditOptions{
			ReplyMarkup: replyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return err
	}
	return nil
}

func Commands(ctx *ext.Context, update *ext.Update, msg *tele.Message) error {
	chat := update.EffectiveChat()
	if !utils.UserIsOwner(msg.SenderID) && chat.IsAUser() {
		if !utils.AllowPM() {
			query := update.CallbackQuery
			_, _ = ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
				Alert:   false,
				QueryID: query.QueryID,
				Message: "Nope!",
			})
			return nil
		}
	}
	text := "```Commands\n" + bot.Commands + "```\n\nAll above commands accept '-h' / '--help' arguments to get more detailed help about each command."

	replyMarkup := markup.InlineKeyboard(
		markup.Row(
			markup.Callback(
				"🔙 Back",
				[]byte("ihelp"),
			),
		),
	)

	_, err := msg.Edit(
		text,
		&tele.EditOptions{
			ReplyMarkup: replyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return err
	}
	return nil
}

func Print(ctx *ext.Context, update *ext.Update) error {
	fmt.Println(update.EffectiveMessage.Text)
	/*fmt.Println(update.EffectiveMessage.ReplyToMessage)
	update.EffectiveMessage.SetRepliedToMessage(context.Background(), ctx.Raw, ctx.PeerStorage)
	fmt.Println(update.EffectiveMessage.ReplyToMessage)*/
	return nil
}
