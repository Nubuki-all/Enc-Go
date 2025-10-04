package callbacks

import (
	"fmt"
	"path/filepath"
	"time"

	//"strings"

	"github.com/celestix/gotgproto/ext"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"

	//"github.com/gotd/td/telegram/message/markup"
	//"github.com/gotd/td/tg"

	//"enc/internal/bot"
	//"enc/internal/logger"
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
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
	return nil
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
	_ = query.Answer("Cancelling "+action+" please wait…", false)
	if err := info.Process.Kill(); err != nil {
		logger.Warn(err)
	}
	info.IsCancelled = true
	info.Canceller = sender
	return nil
}
