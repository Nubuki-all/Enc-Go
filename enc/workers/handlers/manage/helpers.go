// Package manage contains handlers that control bot's behaviour
package manage

import (
	"strconv"
	"strings"

	"github.com/celestix/gotgproto/ext"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/utils"
	"enc/utils/tele"
)

func FCForward(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	fchannel := config.Conf.FChannel
	chatID := msg.ChatID
	if !utils.UserIsOwner(msg.SenderID) && chatID != fchannel {
		return nil
	}
	helpMsg := "Forwards replied message to conf.FCHANNEL"
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}
	var reply string
	repMsgID := msg.ReplyToID
	if fchannel == 0 {
		reply = "`FCHANNEL env not set.`"
	} else if repMsgID != 0 {
		reply = "`Reply with a message to forward to FCHANNEL`"
	}
	if reply != "" {
		_, _ = msg.ReplyText(reply)
		return nil
	}
	_, err := tele.ForwardMessage(chatID, fchannel, repMsgID)
	if err != nil {
		reply = "**Message could not be forwarded:**\nError: " +
			err.Error()
	} else {
		reply = "`Forwarded succesfully.`"
	}
	_, _ = msg.ReplyText(reply)
	return nil
}

func RenameFilterExists(name string, caseSensitive bool) bool {
	for _, filter := range bot.RenameFilters {
		if strings.EqualFold(filter.Match, name) {
			if !caseSensitive && !filter.CaseSensitive {
				return true
			}
			if filter.Match == name {
				return true
			}
		}
	}
	return false
}

func updateMatchFilter(filters []*types.MatchedFilterData, release bool) {
	if release {
		bot.ReleaseFilters = filters
	} else {
		bot.NameFilters = filters
	}
}

func MatchFilterExists(name string, filters []*types.MatchedFilterData) bool {
	for _, filter := range filters {
		if filter.Match == name {
			return true
		}
	}
	return false
}

func ReleaseFilterExists(name string, caseSensitive bool) bool {
	for _, filter := range bot.ReleaseFilters {
		if strings.EqualFold(filter.Match, name) {
			if !caseSensitive && !filter.CaseSensitive {
				return true
			}
			if filter.Match == name {
				return true
			}
		}
	}
	return false
}

func convertStrToMtchFiltr(datas []string) []*types.MatchedFilterData {
	out := []*types.MatchedFilterData{}
	for _, data := range datas {
		values := strings.Split(data, "||")
		valueLen := len(values)
		if valueLen < 2 {
			continue
		}
		fil := &types.MatchedFilterData{
			Match:      values[0],
			CaptionTag: values[1],
		}
		if valueLen > 2 {
			if v, err := strconv.ParseBool(values[2]); err == nil {
				fil.CaseSensitive = v
			}
		}
		if valueLen > 3 {
			if v, err := strconv.ParseBool(values[3]); err == nil {
				fil.MatchOnce = v
			}
		}
		out = append(out, fil)

	}
	return out
}

func ReleaseExtFilterExists(name string, caseSensitive bool) bool {
	for _, filter := range bot.RGExtendedFilters {
		if strings.EqualFold(filter.Match, name) {
			if !caseSensitive && !filter.CaseSensitive {
				return true
			}
			if filter.Match == name {
				return true
			}
		}
	}
	return false
}

func CancelProcesses(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	helpMsg := "Cancels all download/upload/encode processes\n" +
		"Arguments:\n" +
		"  'transfer' - Cancels all ongoing download and upload\n" +
		"  'encode' - Cancels all ongoing (queued) encoding\n" +
		"  'mux' - cancels all ongoing and queued muxing\n" +
		"  'rename' - Cancels all queued rename processes\n" +
		"  'queue' - Clears encoding queue\n" +
		"  'all' Cancels all of the above\n" +
		"   [Default], (No Argument) - Clear queued & current encoding processes.\n" +
		"Supply the args (if any) without single quotes"
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}
	noArgs := args == ""
	bot.ECallbackDataMutex.RLock()
	defer bot.ECallbackDataMutex.RUnlock()
	if noArgs {
		bot.Queue.Clear()
		for _, data := range bot.ECallbackData {
			if data.Process == nil || data.ExtraFlags != "" {
				continue
			}
			_ = data.Process.Kill()
			data.IsCancelled = true
			data.ReturnFlags = "cancel_jobs"
		}
		_, _ = msg.ReplyText("**Cleared all queued encodes and cancelled current encode process(es) (if any)**")
		return nil
	}
	if strings.EqualFold(args, "all") {
		for _, data := range bot.ECallbackData {
			data.IsCancelled = true
			if data.Process != nil {
				_ = data.Process.Kill()
				if data.ExtraFlags == "" {
					data.ReturnFlags = "cancel_jobs"
				}
			}
		}
		bot.Queue.Clear()
		bot.WaitQueueM.Clear()
		bot.WaitQueueR.Clear()
		_, _ = msg.ReplyText("**Cleared all queued items, encode, mux, download, rename and upload processes.**")
		return nil
	}
	if strings.EqualFold(args, "transfer") {
		for _, data := range bot.ECallbackData {
			if data.Process == nil {
				data.IsCancelled = true
			}
		}
		_, _ = msg.ReplyText("**Cancelled all uploads and downloads**")
		return nil
	}
	if strings.EqualFold(args, "encode") {
		for _, data := range bot.ECallbackData {
			if data.Process == nil || data.ExtraFlags != "" {
				continue
			}
			_ = data.Process.Kill()
			data.IsCancelled = true
			data.ReturnFlags = "cancel_jobs"
		}

		_, _ = msg.ReplyText("**Cancelled all encodes succesfully**")
		return nil

	}
	if strings.EqualFold(args, "mux") {
		for _, data := range bot.ECallbackData {
			if data.Process == nil || data.ExtraFlags == "" {
				continue
			}
			_ = data.Process.Kill()
			data.IsCancelled = true
		}
		bot.WaitQueueM.Clear()
		_, _ = msg.ReplyText("**Cancelled all mux processes succesfully**")
		return nil
	}
	if strings.EqualFold(args, "rename") {
		bot.WaitQueueR.Clear()
		_, _ = msg.ReplyText("**Cancelled all queued rename processes succesfully**")
		return nil
	}
	if strings.EqualFold(args, "queue") {
		bot.Queue.Clear()
		_, _ = msg.ReplyText("**Cleared all queued encodes.**")
		return nil
	}
	_, _ = msg.Reply(
		"Unknown args, for help send "+update.Args()[0]+" --help",
		&tele.ReplyOptions{HTMLParseMode: true},
	)
	return nil
}
