package channelstatus

import (
	"fmt"
	"strings"
	"time"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/logger"
	batchutils "enc/utils/batch"
	"enc/utils/parser"
	"enc/utils/tele"
	"enc/workers/auto/transcode"

	"go.uber.org/zap"
)

func getBatchStatusPreview() string {
	entry, found := bot.Queue.GetByIndex(0)
	if !found || len(entry.Batch) == 0 {
		return ""
	}
	next, left := batchutils.GetPendingItems(
		entry.Batch,
		transcode.Einfo.BatchID,
		6,
		entry.AniList,
		entry.Version,
		entry.Filter,
		false,
	)
	if len(next) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("**CURRENTLY QUEUED ITEMS IN BATCH:**\n")
	for i, name := range next {
		fmt.Fprintf(&sb, "%d. `%s`\n", i+1, name)
	}
	if left != 0 {
		fmt.Fprintf(&sb, "__+%d more…__\n", left)
	}
	return sb.String()
}

func getQueueStatusPreview(start int) string {
	if bot.Queue.Len() <= start {
		return ""
	}
	entries := bot.Queue.AsSlice()
	var sb strings.Builder
	for i, entry := range entries {
		if i < start {
			continue
		}
		if i > 6+start {
			fmt.Fprintf(&sb, "__+%d more…__\n", len(entries)-i)
			break
		}
		var err error
		var pName string
		if len(entry.Batch) != 0 {
			pName = "[Batch]:- " + entry.Name
		} else {
			pName, err = parser.QParse(
				entry.Name,
				&parser.ParseOptions{
					Direct:         entry.ForceName,
					UseAniList:     entry.AniList,
					FilterOverride: entry.Filter,
					Version:        entry.Version,
				},
			)
			if err != nil {
				pName = entry.Name
			}
		}
		fmt.Fprintf(&sb, "%d. `%s`\n", i, pName)
	}
	return sb.String()
}

func Start() {
	if config.Conf.FChannel != 0 && config.Conf.FChannelStat != 0 {
		go autoStat()
	}
}

var prevMsg string

func autoStat() {
	ch := make(chan struct{}, 1)
	ch <- struct{}{} // force update on startup
	var lastWake time.Time
	bot.Queue.Subscribe(0, ch)
	for {
		select {
		case <-ch:
			if time.Since(lastWake) < 10*time.Second {
				continue
			}
			lastWake = time.Now()
		case <-transcode.Einfo.Event:
		}
		if bot.Queue.Len() == 0 {
			editStat("**Currently Resting…😑**")
			continue
		}
		var msg string
		start := 0
		if transcode.Einfo.Current != "" {
			msg += fmt.Sprintf("```Currently Encoding\n%s (%s)```\n\n", transcode.Einfo.Current, strings.Join(transcode.Einfo.Codecs(), ","))
			start = 1
		}
		if itms := getBatchStatusPreview(); itms != "" {
			msg += itms + "\n"
		} else {
			msg += getQueueStatusPreview(start)
		}
		editStat(msg)
	}
}

func editStat(msg string) {
	if prevMsg == msg {
		return
	}
	prevMsg = msg
	err := tele.EditMessage(config.Conf.FChannel, config.Conf.FChannelStat, msg)
	if err != nil {
		logger.WarnW("Error editing FChannel status message", zap.Error(err))
	}
}
