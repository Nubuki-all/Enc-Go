package callbacks

import (
	"cmp"
	"fmt"
	"path/filepath"
	//"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"

	//"github.com/gotd/td/telegram/message/markup"
	//"github.com/gotd/td/tg"

	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
	batchutils "enc/utils/batch"
	"enc/utils/tele"
)

func EncodeInfo(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	if info.ExtraFlags == "mux" {
		return query.Answer("Muxing:\n"+filepath.Base(info.OutPath), true)
	}
	total := bot.Queue.Len()
	if total == 0 {
		return query.Answer(
			"Yikes..\nQID 0 is missing, You wouldn't happen to know why? would you?",
			true,
		)
	}
	entry := bot.Queue.AsSlice()[1]
	var ans string
	if info.DlPath != "" {
		ans = "Anilist thumbnail:\nYes"
	}
	length := total -1
	var next1, next2 string
	if len(entry.Batch) > 0 {
		next, left := batchutils.GetPendingItems(
			entry.Batch,
			2,
			entry.AniList,
			entry.Version,
			entry.Filter,
			true,
		)
		nextL := len(next)
		switch nextL {
		case 0:
			next1, next2 = getNext()
		case 1:
			next1 = next[0]
		default:
			next1, next2 = next[0], next[1]
			next1 += "\n"
			if left != 0 {next2 += fmt.Sprintf(" (+%d)", left)}
		}
	} else {
		next1, next2 = getNext()
	}
	if next1 != "" {
		ans = fmt.Sprintf("\n\nNext up:\n%s%s\n\nRemains:- %d", next1, next2, length)
	}
	ans = cmp.Or(ans, "Wow such emptiness 🐱")
	return query.Answer(ans, true)
}

func EncodeStats(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	id := fmt.Sprintf("%d:%d", query.ChatID, query.ID)
	info := utils.GetCallbackInfo(id)
	if info == nil {
		return cleanOldMessage(ctx, update, query)
	}
	args := string(query.Query.Data)
	argL := max(1, len(args))
	data := args[argL-1:]
	outSize := utils.SizeOf(info.OutPath)
	dlSize := utils.SizeOf(info.Path)
	name := filepath.Base(info.Path)
	if len(name) > 45 {
		name = name[:45] + "…"
	}
	du, _ := disk.Usage("/")
	elapsed := time.Since(info.Time).Round(time.Second).Seconds()
	uptime := time.Since(bot.BotStartTime).Round(time.Second).Seconds()
	var cpuUsage float64
	if c, err := cpu.Percent(500*time.Millisecond, false); err == nil {
		cpuUsage = c[0]
	}
	var ans string
	switch data {
	case "0":
		ans = fmt.Sprintf(
			"Downloaded:\n%s\n\nFileName:\n%s\n\nEncoded:\n%s\n\nElapsed time:\n%s",
			utils.HBS(dlSize),
			name,
			utils.HBS(outSize),
			utils.TimeFormatter(elapsed),
		)
	case "1":
		ans = fmt.Sprintf(
			"CPU: %.2f%%\n\nTotal Disk Space:\n%s\n\nBot Uptime:\n%s\n\nUsed: %s  Free: %s",
			cpuUsage,
			utils.GetReadableFileSize(int64(du.Total)),
			utils.TimeFormatter(uptime),
			utils.GetReadableFileSize(int64(du.Used)),
			utils.GetReadableFileSize(int64(du.Free)),
		)
	case "2":
		ans = fmt.Sprintf(
			"CPU: %.2f%%\n\nTotal Disk Space:\n%s\n\nDownloaded:\n%s\n\nEncoded:\n%s\n\nElapsed:\n%s\n\nBot Uptime:\n%s\n\nUsed: %s  Free: %s",
			cpuUsage,
			utils.GetReadableFileSize(int64(du.Total)),
			utils.HBS(dlSize),
			utils.HBS(outSize),
			utils.TimeFormatter(elapsed),
			utils.TimeFormatter(uptime),
			utils.GetReadableFileSize(int64(du.Used)),
			utils.GetReadableFileSize(int64(du.Free)),
		)
	default:
		ans = "Malformed Query!"
	}
	return query.Answer(ans, true)
}

func CancelEncode(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	return cancelEncode(ctx, update, query, false)
}

func CancelEncodeJobs(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery) error {
	return cancelEncode(ctx, update, query, true)
}

func cancelEncode(ctx *ext.Context, update *ext.Update, query *tele.CallbackQuery, cancelJobs bool) error {
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
	action := "encoding"
	if info.ExtraFlags == "mux" {
		action = "muxing"
	}
	if cancelJobs {
		action += " and all pending jobs,"
		info.ReturnFlags = "cancel_jobs"
	}
	_ = query.Answer("Cancelling "+action+" please wait…", false)
	if err := info.Process.Kill(); err != nil {
		logger.Warn(err)
	}
	info.IsCancelled = true
	info.Canceller = sender
	return nil
}	
