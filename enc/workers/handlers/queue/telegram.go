package queue

import (
	"cmp"
	"fmt"
	"slices"
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
	"enc/utils/tele"
	)

const notAVid = "`Replied Message is not a Video.`"

func AddVideo(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	var err error

	helpMsg := "Adds a  Video to encoding queue:\n" +
		"	Requires a reply to the video as argument\n" +
		"	can also add consecutive items to queue by replying\n" +
		"	to the first video and a number of how many subsequent video(s) to add to queue\n" +
		"Accepts the following flags:"
	if msg.ReplyToID == 0 {
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
			"allow-duplicates",
			"A",
			"Allows adding links with the same filename",
		),
		argparser.CreateDBoolFlag(
			"da",
			"",
			"Disables anilist for queued item",
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
	message := update.EffectiveMessage
	err = message.SetRepliedToMessage(ctx, ctx.Raw, ctx.PeerStorage)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	repMsg := message.ReplyToMessage
	if repMsg.Media == nil {
		_, _ = msg.ReplyText(notAVid)
		return nil
	}
	pargs := arg.Values
	var xtraArg string
	if len(pargs) == 0 && !strings.HasPrefix(args, `"`) {
		xtraArg = args
	} else {
		xtraArg = strings.Join(arg.UnknownArgs, " ")
	}	
	forcedFileName := pargs["force"].(string)
	tcaption := pargs["tc"].(string)
	tfile := pargs["tf"].(string)
	remove := pargs["rm"].(string)
	version := pargs["version"].(string)	
	dAnilist := pargs["da"].(bool)
	allowDup := pargs["allow-duplicates"].(bool)	
	qSource := types.QueueSource{}
	qSource.Source = source.Telegram
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
		toSave := false
		var m *tele.Message
		for c, id := 0, msg.ReplyToID; c < i; c, id = c+1, id+1 {
			gtMsgs, err := tele.GetMessages(msg.ChatID, &tg.InputMessageID{ID: id})
			if err != nil {
				_, _ = msg.ReplyText("Could not fetch next message in chain")
				break
			}
			gtMsg := gtMsgs[0]
			pMsg := tele.NewMsgFromGotg(gtMsg, ctx, msg.ChatID, 0)	
			fMeta := utils.MediaToFileMeta(gtMsg.Media)
			if fMeta == nil || !slices.Contains(utils.VideoMimetype, fMeta.MimeType) {
				_, _ = pMsg.ReplyText(notAVid)
				break
			}
			name := fMeta.FileName
			if bot.CaptionParse {
				if gtMsg.Text != "" && !strings.Contains(gtMsg.Text, "\n") {
					name = gtMsg.Text
				}
			}
			if name == "" {
				name = "video_" + time.Now().Format("2006-01-02_15-04-05") + ".mp4"
			}	
			if bot.Queue.Find(name) {
				if !allowDup {
					_, _ = pMsg.ReplyText("**This item has already been added to queue.**")
					continue
				}
				inQueue := true
				for inQueue {
					name = utils.RenameDup(name)
					if bot.Queue.Find(name) {
						inQueue = false
					}
				}
			}
			id := bot.Queue.Add(types.QueueEntry{
				Name:      name,
				ChatID:    msg.ChatID,
				MessageID: pMsg.ID,
				Sender:    msg.SenderID,
				Version:   version,
				Filter:    filter,
				Source:    qSource,
				ForceName: forcedFileName,
				AniList:   !dAnilist,	
				Doc:       fMeta,
			})
			toSave = true
			m, _ = msg.ReplyText("**Video added To Queue ⏰, POS:** `" + strconv.Itoa(id) + "`\n`Please Wait , Encode will start soon`")
		}
		if toSave && bot.Queue.Len() > 1 && m != nil {
			go func() { _ = previewQueue(m, false) }()
		}
		if toSave {
			db.SaveQueue(ctx)
		}
		return nil
	}
	fMeta := utils.MediaToFileMeta(repMsg.Media)	
	if fMeta == nil || !slices.Contains(utils.VideoMimetype, fMeta.MimeType) {
		_, _ = msg.ReplyText(notAVid)
		return nil
	}
	name := fMeta.FileName
	if bot.CaptionParse {
		if repMsg.Text != "" && !strings.Contains(repMsg.Text, "\n") {
			name = repMsg.Text
		}
	}
	if name == "" {
		name = "video_" + time.Now().Format("2006-01-02_15-04-05") + ".mp4"
	}
	if bot.Queue.Find(name) {
		if !allowDup {
			_, _ = msg.ReplyText("**This video has already been added to queue.**")
			return nil
		}
		inQueue := true
		for inQueue {
			name = utils.RenameDup(name)
			if bot.Queue.Find(name) {
				inQueue = false
			}
		}
	}
	id := bot.Queue.Add(types.QueueEntry{
		Name:      name,
		ChatID:    msg.ChatID,
		MessageID: repMsg.ID,
		Sender:    msg.SenderID,
		Version:   version,
		Filter:    filter,
		Source:    qSource,
		ForceName: forcedFileName,
		AniList:   !dAnilist,
		Doc:       fMeta,
	})
	db.SaveQueue(ctx)
	total := bot.Queue.Len()
	if total > 1 || bot.Paused.Any() {
		m, err := msg.ReplyText("**Video added To Queue ⏰, POS:** `" + strconv.Itoa(id) + "`\n`Please Wait , Encode will start soon`")
		if total > 1 && err == nil {
			go func() { _ = previewQueue(m, false) }()
		}
	}
	return nil
}
