package queue

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"enc/internal/bot"
	"enc/internal/bot/source"
	"enc/internal/bot/types"
	"enc/internal/db"

	//"enc/internal/logger"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/batch"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

const (
	invalidMsg = "`Invalid torrent/direct link`"
	noURIMsg   = "`uhm you need to reply to or send command alongside a uri/direct link`"
	NoBatchmsg = "`Link has multiple files.\nTo add all files to queue use -b or -s <id> to choose a single file`"
)

func AddLink(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	var err error

	helpMsg := "Adds a link or torrent link to encoding queue:\n" +
		"	Requires a reply to link or the link as argument\n" +
		"	can also add consecutive items to queue by replying\n" +
		"	to the first link and a number of how many subsequent links to add to queue\n" +
		"Accepts the following flags:"
	if args == "" {
		args = "--help"
	}
	// always parse the args since positional arguments are allowed
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"link",
			"l",
			"Link of file",
		),
		argparser.CreateDStringFlag(
			"force",
			"f",
			"Force rename to file name",
		),
		argparser.CreateDStringFlag(
			"rm",
			"r",
			"Words to remove from filename, delimeted by '|'",
		),
		argparser.CreateDStringFlag(
			"select",
			"s",
			"File id (if link is a folder)",
		),
		argparser.CreateDStringFlag(
			"tc",
			"C",
			"caption_tag (tag caption type as…)",
		),
		argparser.CreateDStringFlag(
			"tf",
			"F",
			"file_tag (tag file_as)",
		),
		argparser.CreateDStringFlag(
			"version",
			"v",
			"tag according to release version number",
		),
		argparser.CreateDBoolFlag(
			"batch",
			"b",
			"Enables adding link with multiple files",
		),
		argparser.CreateDBoolFlag(
			"allow-duplicates",
			"A",
			"Allows adding links with the same filename",
		),
		argparser.CreateDBoolFlag(
			"da",
			"",
			"Disables anilist for queued item",
		),
		argparser.CreateDBoolFlag(
			"confirm",
			"y",
			"(used in conjunction with -b) add all batch to queue without preview",
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
	var xtraArg string
	if len(pargs) == 0 && !strings.HasPrefix(args, `"`) {
		xtraArg = args
	} else {
		xtraArg = strings.Join(arg.UnknownArgs, " ")
	}
	link := pargs["link"].(string)
	if link == "" {
		if msg.ReplyToID != 0 {
			message := update.EffectiveMessage
			err = message.SetRepliedToMessage(ctx, ctx.Raw, ctx.PeerStorage)
			if err != nil {
				_, _ = msg.ReplyText(err.Error())
				return nil
			}
			link = utils.ExtractMessageText(message.ReplyToMessage)
		} else {
			link = xtraArg
		}
	}
	if !utils.IsURL(link) && !utils.IsMagnet(link) {
		_, _ = msg.ReplyText(noURIMsg)
		return nil
	}
	forcedFileName := pargs["force"].(string)
	tcaption := pargs["tc"].(string)
	tfile := pargs["tf"].(string)
	remove := pargs["rm"].(string)
	selected := pargs["tf"].(string)
	version := pargs["version"].(string)
	batch := pargs["batch"].(bool)
	dAnilist := pargs["da"].(bool)
	y := pargs["confirm"].(bool)
	allowDup := pargs["allow-duplicates"].(bool)
	var s int
	if selected != "" && !utils.IsNumber(selected) {
		_, _ = msg.ReplyText("`Value for '-s' arg has to be digit.`")
		return nil
	}
	if selected != "" {
		s, _ = strconv.Atoi(selected)
	}
	qSource := types.QueueSource{}
	var batches []types.BatchItem
	var filter *types.FilterData
	if cmp.Or(remove, tfile, tcaption) != "" {
		filter = &types.FilterData{Fil1: remove, Fil2: tfile, Fil3: tcaption}
	}
	if msg.ReplyToID != 0 && xtraArg != "" {
		if !utils.IsPositiveNumber(xtraArg) {
			_, _ = msg.ReplyText(fmt.Sprintf("**Yeah No.**\n`Error: expected a number but received '%s'.`", xtraArg))
			return nil
		}
		i, _ := strconv.Atoi(xtraArg)
		if batch && !y {
			_, _ = msg.ReplyText("Warning: '-b' flag ignored!\nAdd -y to bypass this.")
			batch = false
		}
		if selected != "" && !y {
			_, _ = msg.ReplyText("Warning: '-s' flag ignored!\nAdd -y to bypass this.")
			selected = ""
		}
		defer db.SaveQueue(ctx)
		var m *tele.Message
		for c, id := 0, msg.ID-1; c < i; c, id = c+1, id+1 {
			gtMsgs, err := tele.GetMessages(msg.ChatID, &tg.InputMessageID{ID: id})
			if err != nil {
				_, _ = msg.ReplyText("Could not fetch next message in chain")
				return nil
			}
			pMsg := tele.NewMsgFromGotg(gtMsgs[0], ctx, msg.ChatID, 0)
			uri := utils.ExtractMessageText(gtMsgs[0])
			if uri == "" {
				_, _ = pMsg.ReplyText("Not a link.")
				return nil
			}
			if !utils.IsURL(uri) && !utils.IsMagnet(uri) {
				_, _ = pMsg.ReplyText(invalidMsg)
				return nil

			}
			var dlInfo *utils.DLInfo
			var mega bool
			qSource := types.QueueSource{}
			if utils.IsMegaLink(uri) {
				mega = true
				qSource.Source = source.Mega
				dlInfo = downloaders.GetMegaDlInfo(uri)
			} else {
				qSource.Source = source.Aria2
				dlInfo = downloaders.GetAria2DlInfo(uri)
			}
			if dlInfo.Error != nil {
				_, _ = pMsg.ReplyText("**Error:** " + dlInfo.Error.Error())
				time.Sleep(5 * time.Second)
				continue
			}
			if dlInfo.Count > 1 && selected == "" && !batch {
				_, _ = pMsg.ReplyText(NoBatchmsg)
				continue
			}
			if selected != "" {
				if !mega {
					s -= 1
				}
				if s > (dlInfo.Count - 1) {
					lastFileID := dlInfo.Count
					if mega {
						lastFileID -= 1
					}
					_, _ = pMsg.ReplyText(fmt.Sprintf("'-s': `%s` is more than last file_id :- `%d`\nTotal files in folder :- `%d`", selected, lastFileID, dlInfo.Count))
					continue
				}
				if name := dlInfo.Files[s]; utils.IsVideoFile(name) {
					dlInfo.Name = name
				} else {
					_, _ = pMsg.ReplyText(fmt.Sprintf("File with name: %s is not a Video", name))
					continue
				}
				qSource.Select = selected
			} else if batch && dlInfo.Count > 1 {
				forcedFileName = ""
				batches = batchutils.GetBatches(dlInfo.Files, mega)
			} else if !utils.IsVideoFile(dlInfo.Name) {
				_, _ = pMsg.ReplyText(fmt.Sprintf("File with name: %s is not a Video", dlInfo.Name))
				continue
			}
			if bot.Queue.Find(dlInfo.Name) {
				if !allowDup {
					_, _ = pMsg.ReplyText("**This item has already been added to queue.**")
					continue
				}
				inQueue := true
				for inQueue {
					dlInfo.Name = utils.RenameDup(dlInfo.Name)
					if bot.Queue.Find(dlInfo.Name) {
						inQueue = false
					}
				}
			}
			bot.Queue.Add(types.QueueEntry{
				Name:      dlInfo.Name,
				ChatID:    msg.ChatID,
				MessageID: msg.ID,
				Sender:    msg.SenderID,
				Version:   version,
				Filter:    filter,
				Source:    qSource,
				ForceName: forcedFileName,
				AniList:   !dAnilist,
				URI:       uri,
				Doc:       nil,
				Batch:     batches,
			})
			total := bot.Queue.Len()
			m, _ = msg.ReplyText("**Link added To Queue ⏰, POS:** `" + strconv.Itoa(total-1) + "`\n`Please Wait , Encode will start soon`")
		}
		if bot.Queue.Len() > 1 && m != nil {
			go func() { _ = previewQueue(m, false) }()
		}
		return nil
	}
	var dlInfo *utils.DLInfo
	var mega bool
	if utils.IsMegaLink(link) {
		mega = true
		qSource.Source = source.Mega
		dlInfo = downloaders.GetMegaDlInfo(link)
	} else {
		qSource.Source = source.Aria2
		dlInfo = downloaders.GetAria2DlInfo(link)
	}
	if dlInfo.Error != nil {
		_, _ = msg.ReplyText("**Error:** " + dlInfo.Error.Error())
		return nil
	}
	if dlInfo.Count > 1 && selected == "" && !batch {
		_, _ = msg.ReplyText(NoBatchmsg)
		return nil
	}
	if selected != "" {
		if !mega {
			s -= 1
		}
		if s > (dlInfo.Count - 1) {
			lastFileID := dlInfo.Count
			if mega {
				lastFileID -= 1
			}
			_, _ = msg.ReplyText(fmt.Sprintf("'-s': `%s` is more than last file_id :- `%d`\nTotal files in folder :- `%d`", selected, lastFileID, dlInfo.Count))
			return nil
		}
		if name := dlInfo.Files[s]; utils.IsVideoFile(name) {
			dlInfo.Name = name
		} else {
			_, _ = msg.ReplyText(fmt.Sprintf("File with name: %s is not a Video", name))
			return nil
		}
		qSource.Select = selected
	} else if batch && dlInfo.Count > 1 {
		forcedFileName = ""
		batches = batchutils.GetBatches(dlInfo.Files, mega)
		if !y {
			batchPreview := batchutils.BatchPreview{
				Bmsg:      msg,
				Version:   version,
				Filter:    filter,
				Items:     batches,
				Previewer: msg.SenderID,
			}
			batches, err = batchPreview.Start(true)
			if err != nil {
				_, _ = msg.ReplyText("**Error:** " + err.Error())
				return nil

			}
			if len(batches) == 0 {
				_, _ = msg.ReplyText("Cancelled adding batch to queue")
				return nil
			}
		}
	} else if !utils.IsVideoFile(dlInfo.Name) {
		_, _ = msg.ReplyText(fmt.Sprintf("File with name: %s is not a Video", dlInfo.Name))
		return nil
	}
	if bot.Queue.Find(dlInfo.Name) {
		if !allowDup {
			_, _ = msg.ReplyText("**This link has already been added to queue.**")
			return nil
		}
		inQueue := true
		for inQueue {
			dlInfo.Name = utils.RenameDup(dlInfo.Name)
			if bot.Queue.Find(dlInfo.Name) {
				inQueue = false
			}
		}
	}
	bot.Queue.Add(types.QueueEntry{
		Name:      dlInfo.Name,
		ChatID:    msg.ChatID,
		MessageID: msg.ID,
		Sender:    msg.SenderID,
		Version:   version,
		Filter:    filter,
		Source:    qSource,
		ForceName: forcedFileName,
		AniList:   !dAnilist,
		URI:       link,
		Doc:       nil,
		Batch:     batches,
	})
	db.SaveQueue(ctx)
	total := bot.Queue.Len()
	if total > 1 || bot.Paused.Any() {
		m, err := msg.ReplyText("**Link added To Queue ⏰, POS:** `" + strconv.Itoa(total-1) + "`\n`Please Wait , Encode will start soon`")
		if total > 1 && err == nil {
			go func() { _ = previewQueue(m, false) }()
		}
	}
	return nil
}
