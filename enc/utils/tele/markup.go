package tele

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/tg"
)

func ToBotMarkup(markup tg.ReplyMarkupClass) interface{} {
	if markup == nil {
		return nil
	}
	switch m := markup.(type) {
	case *tg.ReplyInlineMarkup:
		var rows [][]tgbotapi.InlineKeyboardButton
		for _, row := range m.Rows {
			var buttons []tgbotapi.InlineKeyboardButton
			for _, button := range row.Buttons {
				switch b := button.(type) {
				case *tg.KeyboardButtonCallback:
					buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(b.Text, string(b.Data)))
				case *tg.KeyboardButtonURL:
					buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonURL(b.Text, b.URL))
				}
			}
			if len(buttons) > 0 {
				rows = append(rows, buttons)
			}
		}
		return tgbotapi.NewInlineKeyboardMarkup(rows...)
	case *tg.ReplyKeyboardMarkup:
		var rows [][]tgbotapi.KeyboardButton
		for _, row := range m.Rows {
			var buttons []tgbotapi.KeyboardButton
			for _, button := range row.Buttons {
				switch b := button.(type) {
				case *tg.KeyboardButton:
					buttons = append(buttons, tgbotapi.NewKeyboardButton(b.Text))
				}
			}
			if len(buttons) > 0 {
				rows = append(rows, buttons)
			}
		}
		res := tgbotapi.NewReplyKeyboard(rows...)
		res.ResizeKeyboard = m.Resize
		res.OneTimeKeyboard = m.SingleUse
		res.Selective = m.Selective
		return res
	case *tg.ReplyKeyboardHide:
		return tgbotapi.NewRemoveKeyboard(m.Selective)
	case *tg.ReplyKeyboardForceReply:
		return tgbotapi.ForceReply{ForceReply: true, Selective: m.Selective}
	}
	return nil
}
