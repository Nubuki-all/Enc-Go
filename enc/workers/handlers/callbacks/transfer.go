// Package callbacks contain all callback handlers
package callbacks

import (
	"fmt"
	"path/filepath"
	"time"

	//"strings"

	"github.com/celestix/gotgproto/ext"
	"go.uber.org/zap"

	"github.com/gotd/td/telegram/message/markup"
	//"github.com/gotd/td/tg"

	//"enc/internal/bot"
	//"enc/internal/logger"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/parser"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

const notAllowed = "You're not allowed to do this!"

func CancelDownload(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	_ = query.Answer("Cancelling download please wait…", false)
	info.IsCancelled = true
	info.Canceller = sender
	return nil
}

func CancelUpload(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	_ = query.Answer("Cancelling upload please wait…", false)
	info.IsCancelled = true
	info.Canceller = sender
	return nil
}

func DLStat(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	dlp := info.Path
	fn := filepath.Base(dlp)
	if !utils.FileExists(dlp) {
		dlp += ".!qB"
	}
	sz := utils.SizeOf(dlp)
	entry, found := bot.Queue.GetByIndex(0)
	if found {
		p, err := parser.QParse(fn, &parser.ParseOptions{
			Direct:         entry.ForceName,
			UseAniList:     entry.AniList,
			FilterOverride: entry.Filter,
			Version:        entry.Version,
			Path:           dlp,
		})
		if err != nil {
			fn = p
		} else {
			logger.WarnW("parser returned an error", zap.Error(err))
		}
	}
	ans := fmt.Sprintf(
		"➡️:\n%s\n\nSize:\n%s\n\nElapsed time:\n%s",
		fn,
		utils.HBS(sz),
		utils.TimeFormatter(time.Since(info.Time).Round(time.Second).Seconds()),
	)
	_ = query.Answer(ans, true)
	return nil
}

func ViewDlInfo(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	_ = query.Answer("Please wait…", false)
	if info.PauseProgress {
		return nil
	}
	vidStat := utils.VideoInfo(filepath.Base(info.Path))
	replyMarkup := markup.InlineKeyboard(
		markup.Row(
			markup.Callback(
				"More…",
				[]byte(downloaders.CallbackDataM),
			),
		),
		markup.Row(
			markup.Callback(
				"↩️",
				[]byte(downloaders.CallbackDataB),
			),
		),
		markup.Row(
			markup.Callback(
				"Cancel Download",
				[]byte(downloaders.CallbackData),
			),
		),
	)
	msg := query.GetMessage()
	info.Mutex.Lock()
	defer info.Mutex.Unlock()
	info.PauseProgress = true
	_, _ = msg.Edit(vidStat, &tele.EditOptions{ReplyMarkup: replyMarkup})
	return nil
}

func RevertView(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	sender := query.SenderID
	if info.Sender != sender && !utils.UserIsOwner(sender) {
		_ = query.Answer(notAllowed, false)
		return nil
	}
	_ = query.Answer("Please wait…", false)
	if !info.PauseProgress {
		return nil
	}
	replyMarkup := markup.InlineKeyboard(
		markup.Row(
			markup.Callback(
				"ℹ️",
				[]byte(downloaders.CallbackDataI),
			),
		),
		markup.Row(
			markup.Callback(
				"Cancel Download",
				[]byte(downloaders.CallbackData),
			),
		),
	)
	msg := query.GetMessage()
	info.Mutex.Lock()
	defer info.Mutex.Unlock()
	_, _ = msg.Edit("`...`", &tele.EditOptions{ReplyMarkup: replyMarkup})
	info.PauseProgress = false
	return nil
}
