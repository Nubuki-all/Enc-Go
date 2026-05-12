package tele

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/tg"
)

func ConvertReplyMarkup(markup tg.ReplyMarkupClass) interface{} {
	if markup == nil {
		return nil
	}

	switch m := markup.(type) {
	case *tg.ReplyInlineMarkup:
		var rows [][]tgbotapi.InlineKeyboardButton
		for _, row := range m.Rows {
			var buttons []tgbotapi.InlineKeyboardButton
			for _, btn := range row.Buttons {
				switch b := btn.(type) {
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
		if len(rows) > 0 {
			return tgbotapi.NewInlineKeyboardMarkup(rows...)
		}

	case *tg.ReplyKeyboardMarkup:
		var rows [][]tgbotapi.KeyboardButton
		for _, row := range m.Rows {
			var buttons []tgbotapi.KeyboardButton
			for _, btn := range row.Buttons {
				switch b := btn.(type) {
				case *tg.KeyboardButton:
					buttons = append(buttons, tgbotapi.NewKeyboardButton(b.Text))
				case *tg.KeyboardButtonRequestPhone:
					buttons = append(buttons, tgbotapi.NewKeyboardButtonContact(b.Text))
				case *tg.KeyboardButtonRequestGeoLocation:
					buttons = append(buttons, tgbotapi.NewKeyboardButtonLocation(b.Text))
				}
			}
			if len(buttons) > 0 {
				rows = append(rows, buttons)
			}
		}
		if len(rows) > 0 {
			km := tgbotapi.NewReplyKeyboard(rows...)
			km.ResizeKeyboard = m.Resize
			km.OneTimeKeyboard = m.SingleUse
			km.Selective = m.Selective
			return km
		}

	case *tg.ReplyKeyboardHide:
		return tgbotapi.NewRemoveKeyboard(m.Selective)

	case *tg.ReplyKeyboardForceReply:
		return tgbotapi.ForceReply{
			ForceReply: true,
			Selective:  m.Selective,
		}
	}

	return nil
}
