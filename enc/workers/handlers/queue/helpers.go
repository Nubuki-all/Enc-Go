package queue

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/argparser"
	batchutils "enc/utils/batch"
	"enc/utils/parser"
	queueutils "enc/utils/queue"
	"enc/utils/tele"

	"github.com/celestix/gotgproto/ext"
)

func Select(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	var preview *batchutils.BatchPreview
	if args == "" || msg.ReplyToID == 0 {
		args = "--help"
	} else {
		message := update.EffectiveMessage
		err := message.SetRepliedToMessage(ctx, ctx.Raw, ctx.PeerStorage)
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
		repMsg := message.ReplyToMessage
		id := fmt.Sprintf("%d:%d", utils.GetChatID(repMsg), repMsg.ID)
		p := batchutils.GetBatchCallbackInfo(id)
		if p == nil {
			_, _ = msg.Reply(
				"Replied message is not an active batch preview\nFor help send "+cmd+" --help",
				&tele.ReplyOptions{HTMLParseMode: true},
			)
			return nil
		}
		preview = p.BatchPreview
	}
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "Select files in a batch to encode or skip\n" +
		"Used by replying to an active batch preview\n" +
		"Arguments:"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"enable",
			"e",
			"with a list of file_ids to encode",
		),
		argparser.CreateDStringFlag(
			"disable",
			"d",
			"with a list of file_ids to skip",
		),
		argparser.CreateDBoolFlag(
			"enable-all",
			"E",
			"Select all files in batch.",
		),
		argparser.CreateDBoolFlag(
			"disable-all",
			"D",
			"Deselect all files in batch.",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	disable := pargs["disable"].(string)
	enable := pargs["enable"].(string)
	disableAll := pargs["disable-all"].(bool)
	enableAll := pargs["enable-all"].(bool)
	if disableAll || enableAll {
		txt := "`Selected all files`"
		s := 1
		if disableAll {
			txt = "`Unselected all files.`"
			s = 0
		}
		preview.Mutex.Lock()
		defer preview.Mutex.Unlock()
		var items []types.BatchItem
		for _, item := range preview.Items {
			if item.Status != 3 {
				item.Status = s
			}
			items = append(items, item)
		}
		preview.Items = items
		_, _ = msg.Reply(txt, &tele.ReplyOptions{SleepDelete: 10})
		preview.Update()
		return nil
	}
	if enable == "" && disable == "" {
		_, _ = msg.Reply(
			"Please Supply a Valid argument\nFor help send "+cmd+" --help",
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	}
	preview.Mutex.Lock()
	defer preview.Mutex.Unlock()
	var items []types.BatchItem
	var errs strings.Builder
	var txt string
	if enable != "" {
		h := "**Selected:**\n"
		var enableList []string
		var msg strings.Builder
		if enableList = utils.Range2StringSlice(enable); len(enableList) == 0 {
			sep := " "
			if strings.Contains(enable, ",") {
				sep = ","
			}
			for idx := range strings.SplitSeq(enable, sep) {
				idx = strings.TrimSpace(idx)
				i, err := strconv.Atoi(idx)
				if err != nil || i < 0 {
					errs.WriteString(idx + " - is not a Valid number.\n")
					continue
				}
				enableList = append(enableList, idx)
			}
		}
		for _, item := range preview.Items {
			if !slices.Contains(enableList, item.ID) {
				items = append(items, item)
				continue
			}
			if item.Status == 3 {
				fmt.Fprintf(&errs, "%s - \"%s\" is not a video. Cannot Select.\n", item.ID, item.FileName)
			} else {
				fmt.Fprintf(&msg, "%s - `%s`\n    **Changed from:** '%s'\n\n", item.ID, item.FileName, batchutils.StatusMap[item.Status])
				item.Status = 1
			}
			items = append(items, item)
		}
		if msg.Len() != 0 {
			txt += h
			txt += msg.String() + "\n"
		}
	}
	if disable != "" {
		h := "**Deselected:**\n"
		var disableList []string
		var msg strings.Builder
		if disableList = utils.Range2StringSlice(disable); len(disableList) == 0 {
			sep := " "
			if strings.Contains(disable, ",") {
				sep = ","
			}
			for idx := range strings.SplitSeq(disable, sep) {
				idx = strings.TrimSpace(idx)
				i, err := strconv.Atoi(idx)
				if err != nil || i < 0 {
					errs.WriteString(idx + " - is not a Valid number.\n")
					continue
				}
				disableList = append(disableList, idx)
			}
		}
		for _, item := range preview.Items {
			if !slices.Contains(disableList, item.ID) {
				items = append(items, item)
				continue
			}
			if item.Status == 3 {
				fmt.Fprintf(&errs, "%s - \"%s\" is not a video. Cannot Select.\n", item.ID, item.FileName)
			} else {
				fmt.Fprintf(&msg, "%s - `%s`\n    **Changed from:** '%s'\n\n", item.ID, item.FileName, batchutils.StatusMap[item.Status])
				item.Status = 1
			}
			items = append(items, item)
		}
		if msg.Len() != 0 {
			txt += h
			txt += msg.String()
		}
	}
	if len(txt) != 0 {
		txt += "\n**Errors:**\n" + errs.String()
		for _, stxt := range utils.SplitText(txt, "\n", false) {
			_, _ = msg.Reply(stxt, &tele.ReplyOptions{SleepDelete: 10})
		}
		preview.Items = items
		preview.Update()
	} else {
		txt = "**Errors:**\n" + errs.String()
		_, _ = msg.Reply(txt, &tele.ReplyOptions{SleepDelete: 10})
	}
	return nil
}

func previewQueue(msg *tele.Message, deletable bool) error {
	emptyQueueMsg := "`I'm as free as a bird 🦅`"
	noQueuedMsg := "`Nothing Here, yet! 💖`"
	total := bot.Queue.Len()
	switch total {
	case 0:
		_, _ = msg.Reply(emptyQueueMsg, &tele.ReplyOptions{SleepDelete: 10})
	case 1:
		_, _ = msg.Reply(noQueuedMsg, &tele.ReplyOptions{SleepDelete: 10})
	default:
		preview := queueutils.QueuePreview{Bmsg: msg}
		err := preview.Start()
		if err != nil {
			_, _ = msg.ReplyText("Error: " + err.Error())
		}
	}
	if deletable {
		return msg.Delete()
	}
	return nil
}

func listQueueP(msg *tele.Message, args string) error {
	total := bot.Queue.Len()
	if total == 0 {
		_, _ = msg.Reply("`Nothing In Queue`", &tele.ReplyOptions{SleepDelete: 5})
		return msg.Delete()
	}
	if i, err := strconv.Atoi(args); err == nil {
		if i > (total - 1) {
			_, _ = msg.ReplyText("Supply a Valid queue id/index")
			return nil
		}
		entry := bot.Queue.AsSlice()[i]
		var pName string
		if len(entry.Batch) == 0 {
			pName, _ = parser.QParse(entry.Name, &parser.ParseOptions{
				UseAniList:     entry.AniList,
				FilterOverride: entry.Filter,
				Version:        entry.Version,
				Direct:         entry.ForceName,
			})
		} else {
			pName = "[Batch]: " + entry.Name
		}
		_, _ = msg.ReplyText(strconv.Itoa(i) + ". `" + pName + "`")
		return nil
	}
	ids := utils.Range2Slice(args)
	if len(ids) == 0 {
		_, _ = msg.ReplyText("Send a range of number (i.e 1-5) (max 10 items) or the index number of item in queue to be parsed.")
	}
	if ids[len(ids)-1] > (total - 1) {
		_, _ = msg.ReplyText("Range out of bounds")
		return nil
	}
	var sb strings.Builder
	for i := range ids[:10] {
		entry := bot.Queue.AsSlice()[i]
		var pName string
		if len(entry.Batch) == 0 {
			pName, _ = parser.QParse(entry.Name, &parser.ParseOptions{
				UseAniList:     entry.AniList,
				FilterOverride: entry.Filter,
				Version:        entry.Version,
				Direct:         entry.ForceName,
			})
		} else {
			pName = "[Batch]: " + entry.Name
		}
		sb.WriteString(strconv.Itoa(i) + ". `" + pName + "`")
	}
	if sb.Len() != 0 {
		_, _ = msg.ReplyText("**Parsed Queue Result(s):**\n" + sb.String())
	}
	return nil
}

func ListQueue(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "List items in queue in paged format\n" +
		"with 10 items per page.\n" +
		"Or:"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"parse",
			"p",
			"<int>, range (x-y): Get the auto generated FileName for an item/items in queue",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	parse := pargs["parse"].(string)
	if parse != "" {
		return listQueueP(msg, parse)
	}
	return previewQueue(msg, true)
}

func EditBatch(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	cmd := update.Args()[0]
	helpMsg := "Command for viewing queued batches;\n" +
		"Queue ID of the batch, enables live preview for the batch."
	if args == "" || utils.SendHelp(args) {
		_, _ = msg.Reply(utils.CodeBlock(helpMsg, true), &tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	helpFunc := func(h string) error {
		_, _ = msg.Reply(
			h+"; Need help? Send <code>"+cmd+" --help</code>",
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	}
	var id int
	var err error
	if id, err = strconv.Atoi(args); err != nil {
		return helpFunc("Supplied queue id is not a number.")
	}
	if id > bot.Queue.Len()-1 {
		return helpFunc("Invalid queue id")
	}
	entry := bot.Queue.AsSlice()[id]
	if entry.Sender != msg.SenderID && !utils.UserIsOwner(msg.SenderID) {
		_, _ = msg.ReplyText("`This item wasn't added to queue by you.`")
		return nil
	}
	if len(entry.Batch) == 0 {
		_, _ = msg.ReplyText("Item with given queue ID is not a batch.")
		return nil
	}
	previewBatch := batchutils.BatchPreview{
		Bmsg:      msg,
		Version:   entry.Version,
		Filter:    entry.Filter,
		Items:     entry.Batch,
		Previewer: msg.SenderID,
	}
	items, err := previewBatch.Start(true)
	if err != nil {
		_, _ = msg.ReplyText("Error: " + err.Error())
		return nil
	}
	if len(items) == 0 {
		return msg.Delete()
	} else {
		_, _ = msg.ReplyText("Edited batch:- `" + entry.Name + "` successfully!")
		db.SaveQueue(ctx)
		return nil
	}
}

func EditQueue(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "Edit configuration of items already in queue:\n" +
		"Be aware that this operation is quite slow\n" +
		"Arguments:"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"name",
			"n",
			"Change Name of queued item.",
		),
		argparser.CreateDStringFlag(
			"force",
			"F",
			"Change Forced name. (no-op on batches)",
		),
		argparser.CreateDStringFlag(
			"qid",
			"q",
			"id of queue item to edit; inferred",
		),
		argparser.CreateDStringFlag(
			"rm",
			"",
			"<filter> remove maches from FileName",
		),
		argparser.CreateDStringFlag(
			"tc",
			"",
			"Force tag caption as -",
		),
		argparser.CreateDStringFlag(
			"tf",
			"",
			"Force tag file as -",
		),
		argparser.CreateDStringFlag(
			"uri",
			"u",
			"Change URI (Not advised)",
		),
		argparser.CreateDStringFlag(
			"version",
			"v",
			"Change version tag; also accepts None to disable",
		),
		argparser.CreateDBoolFlag(
			"disable-filter",
			"D",
			"Unset filters for queue",
		),
		argparser.CreateDBoolFlag(
			"disable-anilist",
			"d",
			"disables AniList parsing for this item",
		),
		argparser.CreateDBoolFlag(
			"enable-anilist",
			"e",
			"enables AniList parsing for this item",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpFunc := func(h string) error {
		_, _ = msg.Reply(
			h+"; Need help? Send <code>"+cmd+" --help</code>",
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	}
	pargs := arg.Values
	if len(pargs) == 0 {
		return helpFunc("Invalid Argument")
	}
	qid := pargs["qid"].(string)
	if qid == "" {
		qid = strings.Join(arg.UnknownArgs, " ")
	}
	if qid == "" {
		return helpFunc("Please Supply a queue id")
	}
	var id int
	if i, err := strconv.Atoi(qid); err != nil {
		return helpFunc("Supplied queue id is not a number.")
	} else {
		id = i
	}
	if id > bot.Queue.Len()-1 {
		return helpFunc("Invalid queue id")
	}
	entry := bot.Queue.AsSlice()[id]
	if entry.Sender != msg.SenderID && !utils.UserIsOwner(msg.SenderID) {
		_, _ = msg.ReplyText("`This item wasn't added to queue by you.`")
		return nil

	}
	name := pargs["name"].(string)
	forcedName := pargs["force"].(string)
	remove := pargs["rm"].(string)
	tcap := pargs["tc"].(string)
	tfile := pargs["tf"].(string)
	uri := pargs["uri"].(string)
	version := pargs["version"].(string)
	da := pargs["disable-anilist"].(bool)
	df := pargs["disable-filter"].(bool)
	ea := pargs["enable-anilist"].(bool)
	if cmp.Or(remove, tcap, tfile) != "" {
		if entry.Filter == nil {
			entry.Filter = &types.FilterData{}
		}
		entry.Filter.Fil1 = cmp.Or(remove, entry.Filter.Fil1)
		entry.Filter.Fil2 = cmp.Or(tfile, entry.Filter.Fil2)
		entry.Filter.Fil3 = cmp.Or(tcap, entry.Filter.Fil3)
	} else if df {
		entry.Filter = nil
	}
	if ea {
		entry.AniList = ea
	} else if da {
		entry.AniList = !da
	}
	if version == "None" {
		entry.Version = ""
	} else {
		entry.Version = cmp.Or(version, entry.Version)
	}
	entry.URI = cmp.Or(uri, entry.URI)
	entry.ForceName = cmp.Or(forcedName, entry.ForceName)
	if name == "" {
		bot.Queue.Update(entry)
	} else {
		prevName := entry.Name
		entry.Name = name
		bot.Queue.Replace(prevName, entry)
	}
	db.SaveQueue(ctx)
	_, _ = msg.ReplyText("`Edited queued item successfully.`")
	return nil
}

func ClearQueue(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	helpMsg := "Clears an item or all items from queue\n" +
		"Arguments:\n" +
		"-  A number or a range of numbers (queue ids) to remove\n" +
		"-  'all' (not case sensitive) to 'pending' queue items\n" +
		"-  '0' Remove the topmost queued item (QID 0) (not recommended)"
	cmd := update.Args()[0]
	if args == "" || utils.SendHelp(args) {
		_, _ = msg.Reply(utils.CodeBlock(helpMsg, true), &tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	helpFunc := func(h string) error {
		_, _ = msg.Reply(
			h+"; Need help? Send <code>"+cmd+" --help</code>",
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	}
	btchClrMsg := "**Cleared the following items from queue:**\n"
	var id int
	var err error
	if id, err = strconv.Atoi(args); err == nil {
		if id > bot.Queue.Len()-1 {
			return helpFunc("Invalid queue id")
		}
		entry := bot.Queue.AsSlice()[id]
		if entry.Sender != msg.SenderID && !utils.UserIsOwner(msg.SenderID) {
			_, _ = msg.ReplyText("`This item wasn't added to queue by you.`")
			return nil
		}
		bot.Queue.Remove(id)
		_, _ = msg.ReplyText("`" + entry.Name + "` has been removed from queue")
		return nil
	}
	if strings.EqualFold(args, "all") {
		total := bot.Queue.Len()
		if total < 2 {
			_, _ = msg.ReplyText("`Nothing to clear...`")
			return nil
		}
		var indexes []int
		for i := 1; i < total; i++ {
			indexes = append(indexes, i)
		}
		sender := msg.SenderID
		if utils.UserIsOwner(sender) {
			sender = 0
		}
		txt := bot.Queue.RemoveItems(indexes, sender, true)
		if len(txt) == 0 {
			_, _ = msg.ReplyText("`Nothing was cleared.`")
		} else {
			_, _ = msg.ReplyText(btchClrMsg + txt)
			db.SaveQueue(ctx)
		}
		return nil
	}
	indexes := utils.Range2Slice(args)
	total := bot.Queue.Len()
	if len(indexes) == 0 {
		return helpFunc("`Pass a number/range/all for an item/items on queue to be removed")
	}
	if indexes[len(indexes)-1] > (total - 1) {
		return helpFunc("Range out of bounds")
	}
	sender := msg.SenderID
	if utils.UserIsOwner(sender) {
		sender = 0
	}
	txt := bot.Queue.RemoveItems(indexes, sender, true)
	if len(txt) == 0 {
		_, _ = msg.ReplyText("`Nothing was cleared.`")
	} else {
		_, _ = msg.ReplyText(btchClrMsg + txt)
		db.SaveQueue(ctx)
	}
	return nil
}
