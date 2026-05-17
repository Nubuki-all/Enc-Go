package callbacks

import (
	//"fmt"
	//"strings"
	"fmt"
	"time"

	//"github.com/gotd/td/telegram/message/markup"
	//"github.com/gotd/td/tg"

	"enc/internal/bot"
	//"enc/internal/logger"
	//"enc/utils"
	"enc/utils/parser"
	"enc/utils/tele"
)

func cleanOldMessage(query *tele.CallbackQuery error {
	_ = query.Answer("⛔ Old button.  Removing…", false)
	time.Sleep(5 * time.Second)
	err := query.GetMessage().Delete()
	return err
}

func getNext() (n1 string, n2 string) {
	queue := bot.Queue.AsSlice()
	total := len(queue)
	var err error
	if total > 1 {
		entry := queue[1]
		if len(entry.Batch) == 0 {
			n1, err = parser.QParse(
				entry.Name,
				&parser.ParseOptions{
					Direct:         entry.ForceName,
					UseAniList:     entry.AniList,
					FilterOverride: entry.Filter,
					Version:        entry.Version,
				},
			)
			if err != nil {
				n1 = entry.Name
			}
		} else {
			n1 = "[Batch]: " + entry.Name
		}
		if len(n1) > 45 {
			n1 = n1[:45] + "…"
		}
	}
	if total > 2 {
		entry := queue[2]
		if len(entry.Batch) == 0 {
			n2, err = parser.QParse(
				entry.Name,
				&parser.ParseOptions{
					Direct:         entry.ForceName,
					UseAniList:     entry.AniList,
					FilterOverride: entry.Filter,
					Version:        entry.Version,
				},
			)
			if err != nil {
				n1 = entry.Name
			}
		} else {
			n2 = "[Batch]: " + entry.Name
		}
		if len(n2) > 45 {
			n2 = n2[:45] + "…"
		}
		if total > 3 {
			n2 += fmt.Sprintf(" (+%d)", total-3)
		}
	}
	return n1, n2
}
