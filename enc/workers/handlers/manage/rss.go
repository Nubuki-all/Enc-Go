package manage

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/db"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/tele"
	"enc/workers/auto/rssmonitor"

	"github.com/celestix/gotgproto/ext"
	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"
)

func RssHandler(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	var err error
	helpMsg := "Base command for rss:\n" +
		"    for additional help send the above arguments with -h/--help or without additional params.\n" +
		"    *Arguments:\n" +
		"    *listed in the order priority.\n"
	if args == "" {
		args = "--help"
	}
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDBoolFlag("delete", "d", "To delete already an subscribed feed."),
		argparser.CreateDBoolFlag("edit", "e", "To edit configurations for already subscribed rss feed."),
		argparser.CreateDBoolFlag("get", "g", "To get previous feeds for given TITLE"),
		argparser.CreateDBoolFlag("list-chat", "c", "To get all chats for given rss feed by TITLE"),
		argparser.CreateDBoolFlag("list", "l", "To list all subscribed feeds."),
		argparser.CreateDBoolFlag("rollback", "r", "To rollback a rss feed."),
		argparser.CreateDBoolFlag("subscribe", "s", "To subscribe to a rss feed."),
	)
	pargs := arg.Values
	del := pargs["delete"].(bool)
	edit := pargs["edit"].(bool)
	get := pargs["get"].(bool)
	list := pargs["list"].(bool)
	sub := pargs["subscribe"].(bool)
	lc := pargs["list-chat"].(bool)
	rollback := pargs["rollback"].(bool)
	if !del && !edit && !get && !list && !sub && !lc && !rollback {
		if out := arg.Output; out != "" {
			_, _ = msg.Reply(
				utils.CodeBlock(out, true),
				&tele.ReplyOptions{HTMLParseMode: true},
			)
			return nil
		} else if err := arg.Err; err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
		txt := fmt.Sprintf("%s requires an argument; send %s --help if lost.", cmd, cmd)
		_, _ = msg.Reply(txt, &tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	if del {
		return delRss(msg, strings.Join(arg.UnknownArgs, " "))
	}
	if sub {
		return subRss(msg, args)
	}
	if edit {
		return editRss(msg, args)
	}
	if get {
		return getRss(msg, args)
	}
	if rollback {
		return rollbackRss(msg, args)
	}
	if list {
		return listRss(msg, "")
	}
	if lc {
		return listRssChat(msg, strings.Join(arg.UnknownArgs, ""))
	}

	return nil
}

func delRss(msg *tele.Message, args string) error {
	helpMsg := "Removes feed with designated title from list of subscribed feeds\n" +
		"    Args:\n" +
		"      TITLE (str): subscribed rss feed title to remove\n" +
		"    Returns:\n" +
		"       Success message on successfull removal\n" +
		"        Not found message if TITLE passed was not found\n"
	if args == "" || utils.SendHelp(args) {
		_, _ = msg.Reply(utils.CodeBlock(helpMsg, true), &tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	bot.RSSDictMutex.RLock()
	_, found := bot.RSSDict[args]
	bot.RSSDictMutex.RUnlock()
	if !found {
		_, _ = msg.ReplyText(fmt.Sprintf("'%s' not found in list of subscribed rss feeds!", args))
		return nil

	}
	bot.RSSDictMutex.Lock()
	delete(bot.RSSDict, args)
	bot.RSSDictMutex.Unlock()
	_, _ = msg.ReplyText(fmt.Sprintf(
		"Succesfully removed '%s' from subscribed feeds!",
		args,
	))
	db.SaveRSSDict(context.Background())
	return nil
}

func subRss(msg *tele.Message, args string) error {
	helpMsg := "Subscribe rss feeds!\n" +
		"simply pass the rss link with the following arguments:"
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	arg := argparser.ParseFlags(
		"rss",
		helpMsg,
		argList,
		argparser.CreateDBoolFlag("subscribe", "s", "HEAD PARAM"),
		argparser.CreateDStringFlag("title", "t", "New Title of the subscribed rss feed [Required]."),
		argparser.CreateDStringFlag("cmd", "c", "A command among ['leech', 'qbleech', 'ytdl'] (no short forms), to pass rss link to bot."),
		argparser.CreateDStringSliceFlag("exf", "e", "Keyword of words to fiter out; Format: 'x || y'; where '||' means on either of both values (OR); for AND use the flag again."),
		argparser.CreateDStringSliceFlag("inf", "i", "Keywords to include; same format as exf"),
		argparser.CreateDInt64SliceFlag("chat-id", "", "chat-id to send feeds."),
		argparser.CreateDStringSliceFlag("chat", "", "'.' for current chat, '{chat_id}:{topic_id}' for topics and 'default' which is the same as not specifying; defaults to RSS_CHAT"),
		argparser.CreateDBoolFlag("nodirect", "", "To disable rss message getting passed directly* to bot"),
		argparser.CreateDBoolFlag("direct", "", "To enable rss message getting passed directly* to bot"),
		argparser.CreateDBoolFlag("pause", "p", "To pause the rss feed."),
		argparser.CreateDBoolFlag("match-both", "", "Treat an item as already seen only if its title and link both match (catches re-releases with the same title)."),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(
			utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	link := strings.Join(arg.UnknownArgs, " ")
	if link == "" || !utils.IsURL(link) {
		_, _ = msg.ReplyText("Invalid RSS URL.")
		return nil
	}
	pargs := arg.Values
	title := pargs["title"].(string)
	if title == "" {
		_, _ = msg.ReplyText("Title is Required. Append --help for help")
		return nil
	}
	cmd := pargs["cmd"].(string)
	exf := pargs["exf"].([]string)
	inf := pargs["inf"].([]string)
	chatID := pargs["chat-id"].([]int64)
	chat := pargs["chat"].([]string)
	nodirect := pargs["nodirect"].(bool)
	direct := pargs["direct"].(bool)
	pause := pargs["pause"].(bool)
	matchBoth := pargs["match-both"].(bool)
	if cmd != "" && !strings.HasPrefix(cmd, "/") {
		cmd = "/" + cmd
	}
	var chats []types.FeedChat
	for _, chatid := range chatID {
		if slices.ContainsFunc(chats, func(fc types.FeedChat) bool {
			return fc.ChatID == chatid
		}) {
			logger.WarnW("Duplicate ChatID @rssSub", zap.Int64("ChatID", chatid))
		}
		chats = append(chats, types.FeedChat{ChatID: chatid})
	}
	var current, def bool
	for _, c := range chat {
		var fc types.FeedChat
		if strings.EqualFold(c, "default") && !def {
			fc.Default = true
			def = true
		} else if strings.EqualFold(c, ".") && !current {
			fc.ChatID = msg.ChatID
			fc.TopicID = msg.TopicID
			current = true
		} else if chatTop := strings.Split(c, ":"); len(chatTop) == 2 {
			chatID, err := strconv.ParseInt(chatTop[0], 10, 64)
			if err == nil {
				if i, err := strconv.Atoi(chatTop[1]); err == nil {
					fc.TopicID = i
					fc.ChatID = chatID
				} else {
					_, _ = msg.ReplyText("Failed extracting TopicID from: " + c + " skipping!")
					continue
				}
			} else {
				_, _ = msg.ReplyText(c + ": Not Valid; skipping.")
				continue
			}
		} else {
			logger.WarnW("Chat was not added!", zap.String("Chat", c))
			continue
		}
		chats = append(chats, fc)
	}
	if len(chats) == 0 {
		chats = append(chats, types.FeedChat{Default: true})
	}
	var exfSlice, infSlice [][]string
	for _, x := range exf {
		exfSlice = append(exfSlice, strings.Split(x, " || "))
	}
	for _, x := range inf {
		infSlice = append(infSlice, strings.Split(x, " || "))
	}
	if nodirect {
		direct = false
	} else if !direct {
		direct = config.Conf.RSSDirect
	}
	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(link)
	if err != nil || len(feed.Items) == 0 || feed.Items[0].Title == "" {
		_, _ = msg.Reply(
			fmt.Sprintf("The link: %s doesn't seem to be a RSS feed or it's region-blocked!", link),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	}
	txt := "<b>Subscribed!</b>"
	txt += fmt.Sprintf(
		"\n<b>Title: </b><code>%s</code>\n<b>Feed Url: </b>%s",
		title,
		link,
	)
	txt += fmt.Sprintf(
		"\n<b>latest record for </b>%s:",
		feed.Title,
	)
	txt += fmt.Sprintf(
		"\nName: <code>%s</code>",
		strings.ReplaceAll(
			strings.ReplaceAll(feed.Items[0].Title, ">", ""),
			"<",
			"",
		),
	)
	txt += "\nLink:- <code>" + feed.Items[0].Link + "</code>"
	bot.RSSDictMutex.Lock()
	bot.RSSDict[title] = &types.FeedData{
		Link:      link,
		LastFeed:  rssmonitor.ItemLink(feed.Items[0]),
		LastTitle: feed.Items[0].Title,
		Chat:      chats,
		Command:   cmd,
		Direct:    direct,
		MatchBoth: matchBoth,
		Exf:       exfSlice,
		Inf:       infSlice,
		Paused:    pause,
	}
	bot.RSSDictMutex.Unlock()
	db.SaveRSSDict(context.Background())
	_, _ = msg.Reply(txt,
		&tele.ReplyOptions{HTMLParseMode: true})
	logger.InfoW(
		"RSS Feed added:",
		zap.Int64("By", msg.SenderID),
		zap.String("Title", title),
		zap.String("Link", link),
		zap.String("Command", cmd),
		zap.Bool("Direct", direct),
		zap.Bool("Paused", pause),
	)
	if pause {
		return nil
	}
	if rssmonitor.DefaultScheduler != nil {
		if rssmonitor.DefaultScheduler.IsPaused() {
			rssmonitor.ResumeRSS()
		}
	}
	return nil
}

func editRss(msg *tele.Message, args string) error {
	helpMsg := "Edit subscribed rss feeds!\n" +
		"simply pass the rss title with the following arguments:"
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	arg := argparser.ParseFlags(
		"rss",
		helpMsg,
		argList,
		argparser.CreateDBoolFlag("edit", "e", "HEAD PARAM"),
		argparser.CreateDStringFlag("link", "l", "New link of the previously subscribed rss feed."),
		argparser.CreateDStringFlag("cmd", "c", "A command among ['leech', 'qbleech', 'ytdl'] (no short forms), to pass rss link to bot."),
		argparser.CreateDStringSliceFlag("exf", "x", "Keyword of words to fiter out; Format: 'x || y'; where '||' means on either of both values (OR); for AND use the flag again."),
		argparser.CreateDStringSliceFlag("inf", "i", "Keywords to include; same format as exf"),
		argparser.CreateDInt64SliceFlag("chat-id", "", "chat-id to send feeds."),
		argparser.CreateDString2SliceFlag("rm-chat", "", "Index of chats to remove; check with list-chat."),
		argparser.CreateDStringSliceFlag("chat", "", "'.' for current chat, '{chat_id}:{topic_id}' for topics and 'default'; defaults to RSS_CHAT"),
		argparser.CreateDBoolFlag("nodirect", "", "To disable rss message getting passed directly* to bot"),
		argparser.CreateDBoolFlag("direct", "", "To enable rss message getting passed directly* to bot"),
		argparser.CreateDBoolFlag("pause", "p", "To pause the rss feed."),
		argparser.CreateDBoolFlag("resume", "r", "To resume the rss feed."),
		argparser.CreateDBoolFlag("match-both", "", "Treat an item as already seen only if its title and link both match (catches re-releases with the same title)."),
		argparser.CreateDBoolFlag("match-either", "", "Treat an item as already seen if its title or link matches [default]."),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(
			utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	title := strings.Join(arg.UnknownArgs, " ")
	title = strings.TrimSpace(title)
	if title == "" {
		_, _ = msg.ReplyText("Title is Required. Append --help for help")
		return nil
	}
	bot.RSSDictMutex.RLock()
	rssData, found := bot.RSSDict[title]
	bot.RSSDictMutex.RUnlock()
	if !found {
		_, _ = msg.ReplyText(
			"Title: " + title +
				" was not found in subscribed feeds.",
		)
		return nil
	}
	pargs := arg.Values
	link := pargs["link"].(string)
	if link != "" && !utils.IsURL(link) {
		_, _ = msg.ReplyText("Invalid RSS URL.")
		return nil
	}
	cmd := pargs["cmd"].(string)
	exf := pargs["exf"].([]string)
	inf := pargs["inf"].([]string)
	chatID := pargs["chat-id"].([]int64)
	chat := pargs["chat"].([]string)
	nodirect := pargs["nodirect"].(bool)
	direct := pargs["direct"].(bool)
	pause := pargs["pause"].(bool)
	resume := pargs["resume"].(bool)
	matchBoth := pargs["match-both"].(bool)
	matchEither := pargs["match-either"].(bool)
	rmChats := pargs["rm-chat"].([]string)
	if cmd != "" && !strings.HasPrefix(cmd, "/") {
		cmd = "/" + cmd
	}
	func() {
		bot.RSSDictMutex.Lock()
		defer bot.RSSDictMutex.Unlock()
		rssData.Command = cmp.Or(cmd, rssData.Command)
		rssData.Link = cmp.Or(link, rssData.Link)
		if len(rmChats) != 0 {
			var chatsIdx []int
			for _, str := range rmChats {
				if i, err := strconv.Atoi(str); err == nil {
					chatsIdx = append(chatsIdx, i)
				}
			}
			rssData.Chat = utils.DeleteByIndices(rssData.Chat, chatsIdx)
		}
		for _, chatid := range chatID {
			if slices.ContainsFunc(rssData.Chat, func(fc types.FeedChat) bool {
				return fc.ChatID == chatid
			}) {
				logger.WarnW("Duplicate ChatID @rssEdit", zap.Int64("ChatID", chatid))
			}
			rssData.Chat = append(rssData.Chat, types.FeedChat{ChatID: chatid})
		}
		var current, def bool
		for _, c := range chat {
			var fc types.FeedChat
			if strings.EqualFold(c, "default") && !def {
				fc.Default = true
				def = true
			} else if strings.EqualFold(c, ".") && !current {
				fc.ChatID = msg.ChatID
				fc.TopicID = msg.TopicID
				current = true
			} else if chatTop := strings.Split(c, ":"); len(chatTop) == 2 {
				chatID, err := strconv.ParseInt(chatTop[0], 10, 64)
				if err == nil {
					if i, err := strconv.Atoi(chatTop[1]); err == nil {
						fc.TopicID = i
						fc.ChatID = chatID
					} else {
						_, _ = msg.ReplyText("Failed extracting TopicID from: " + c + " skipping!")
						continue
					}
				} else {
					_, _ = msg.ReplyText(c + ": Not Valid; skipping.")
					continue
				}
			} else {
				logger.WarnW("Chat was not added!", zap.String("Chat", c))
				continue
			}
			rssData.Chat = append(rssData.Chat, fc)
		}

		var exfSlice, infSlice [][]string
		for _, x := range exf {
			exfSlice = append(exfSlice, strings.Split(x, " || "))
		}
		for _, x := range inf {
			infSlice = append(infSlice, strings.Split(x, " || "))
		}
		if len(exfSlice) != 0 {
			rssData.Exf = exfSlice
		}
		if len(infSlice) != 0 {
			rssData.Inf = infSlice
		}
		if nodirect {
			rssData.Direct = false
		} else {
			rssData.Direct = cmp.Or(rssData.Direct, direct)
		}
		if matchEither {
			rssData.MatchBoth = false
		} else if matchBoth {
			rssData.MatchBoth = true
		}
		if resume && rssData.Paused {
			pause = false
		}
		rssData.Paused = pause
	}()

	_, _ = msg.Reply("Updated Rss configuration successfully.",
		&tele.ReplyOptions{HTMLParseMode: true})
	db.SaveRSSDict(context.Background())

	logger.InfoW(
		"RSS Feed Updated:",
		zap.Int64("By", msg.SenderID),
		zap.String("Title", title),
		zap.String("Link", rssData.Link),
		zap.String("Command", rssData.Command),
		zap.Bool("Direct", rssData.Direct),
		zap.Bool("MatchBoth", rssData.MatchBoth),
		zap.Bool("Paused", rssData.Paused),
	)
	if pause {
		return nil
	}
	if rssmonitor.DefaultScheduler != nil {
		if rssmonitor.DefaultScheduler.IsPaused() {
			rssmonitor.ResumeRSS()
		}
	}
	return nil
}

func getRss(msg *tele.Message, args string) error {
	helpMsg := "Get the links of titles in rss!\n" +
		"simply pass the rss title with the following arguments:"
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	arg := argparser.ParseFlags(
		"rss",
		helpMsg,
		argList,
		argparser.CreateDBoolFlag("get", "g", "HEAD PARAM"),
		argparser.CreateDIntFlag("amount", "a", "Amount of links to grab/fetch."),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(
			utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	title := strings.Join(arg.UnknownArgs, " ")
	title = strings.TrimSpace(title)
	if title == "" {
		_, _ = msg.ReplyText("Title is Required. Append --help for help")
		return nil
	}
	bot.RSSDictMutex.RLock()
	rssData, found := bot.RSSDict[title]
	bot.RSSDictMutex.RUnlock()
	if !found {
		_, _ = msg.ReplyText(
			"Title: " + title +
				" was not found in subscribed feeds.",
		)
		return nil
	}
	pargs := arg.Values
	amount := pargs["amount"].(int)
	if amount < 1 {
		_, _ = msg.ReplyText("-a: Supply a valid digit.")
		return nil
	}
	sMsg, err := msg.Reply(
		fmt.Sprintf("Getting the last <b>%d</b> item(s) from %s...", amount, title),
		&tele.ReplyOptions{HTMLParseMode: true},
	)
	if err != nil {
		return nil
	}
	recMsg := sMsg
	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(rssData.Link)
	if err != nil {
		_, _ = sMsg.Edit("<b>Could not fetch feeds from rss link: "+err.Error(), &tele.EditOptions{HTMLParseMode: true})
		return nil
	}
	maxEntries := len(feed.Items)
	var sb strings.Builder
	for _, entry := range feed.Items[:min(maxEntries, amount)] {
		fmt.Fprintf(
			&sb, "<b>Name: </b><code>%s</code>\n",
			strings.ReplaceAll(
				strings.ReplaceAll(entry.Title, "<", ""),
				">",
				"",
			),
		)
		fmt.Fprintf(&sb, "<b>Link: </b><code>%s</code>\n\n", entry.Link)
	}
	for _, txt := range utils.SplitText(sb.String(), "\n\n", false) {
		recMsg, err = recMsg.Reply(txt, &tele.ReplyOptions{HTMLParseMode: true})
		if err != nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	_, _ = sMsg.Edit(
		fmt.Sprintf("Here are the last <b>%d</b> item(s) from %s:", amount, title),
		&tele.EditOptions{HTMLParseMode: true},
	)
	return nil
}

func listRss(msg *tele.Message, args string) error {
	helpMsg := "Get list of subscribed rss feeds\n" +
		"    Args:\n" +
		"      None\n"
	if utils.SendHelp(args) {
		_, _ = msg.Reply(utils.CodeBlock(helpMsg, true), &tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	bot.RSSDictMutex.RLock()
	total := len(bot.RSSDict)
	bot.RSSDictMutex.RUnlock()
	if total == 0 {
		_, _ = msg.Reply("<b> No subscriptions!</b>", &tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	i := 1
	var sb strings.Builder
	parseFilter := func(ftr [][]string) string {
		if len(ftr) == 0 {
			return ""
		}
		outerElements := make([]string, 0, len(ftr))
		for _, sublist := range ftr {
			innerStr := strings.Join(sublist, ", ")
			formattedSublist := fmt.Sprintf("(%s)", innerStr)
			outerElements = append(outerElements, formattedSublist)
		}
		result := strings.Join(outerElements, ", ")
		return result
	}
	bot.RSSDictMutex.RLock()
	for title, data := range bot.RSSDict {
		fmt.Fprintf(&sb, "\n\n%d.<b>Title:</b> <code>%s</code>\n<b>Feed Url: </b><code>%s</code>\n", i, title, data.Link)
		fmt.Fprintf(&sb, "<b>Chats:</b> <code>(%d)</code>\n", len(data.Chat))
		fmt.Fprintf(&sb, "<b>Command:</b> <code>%s</code>\n", data.Command)
		fmt.Fprintf(&sb, "<b>Direct:</b> <code>%v</code>\n", data.Direct)
		fmt.Fprintf(&sb, "<b>Match both title and link:</b> <code>%v</code>\n", data.MatchBoth)
		fmt.Fprintf(&sb, "<b>Include filter:</b> <code>%s</code>\n", parseFilter(data.Inf))
		fmt.Fprintf(&sb, "<b>Exclude filter:</b> <code>%s</code>\n", parseFilter(data.Exf))
		fmt.Fprintf(&sb, "<b>Paused:</b> <code>%v</code>", data.Paused)
		i++
	}
	bot.RSSDictMutex.RUnlock()

	var err error
	recMsg := msg
	for i, txt := range utils.SplitText(sb.String(), "\n\n", true) {
		txt = "<b>Your subscriptions</b> #" + strconv.Itoa(i+1) + txt
		recMsg, err = recMsg.Reply(txt, &tele.ReplyOptions{HTMLParseMode: true})
		if err != nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func listRssChat(msg *tele.Message, args string) error {
	helpMsg := "Get the chats of subscribed rss feeds!\n" +
		"simply pass the rss title."
	if args == "" || utils.SendHelp(args) {
		_, _ = msg.Reply(utils.CodeBlock(helpMsg, true), &tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	bot.RSSDictMutex.RLock()
	rssData, found := bot.RSSDict[args]
	bot.RSSDictMutex.RUnlock()
	if !found {
		_, _ = msg.ReplyText(
			"Title: " + args +
				" was not found in subscribed feeds.",
		)
		return nil
	}
	var sb strings.Builder
	for i, chat := range rssData.Chat {
		var txt string
		if chat.Default {
			txt = "Default: Yes"
		} else if chat.TopicID != 0 {
			txt = fmt.Sprintf("ChatID: %d, TopicID: %d", chat.ChatID, chat.TopicID)
		} else {
			txt = fmt.Sprintf("ChatID: %d", chat.ChatID)
		}
		fmt.Fprintf(&sb, "\n%d. <code>%s</code>", i, txt)
	}
	var err error
	recMsg := msg
	for _, txt := range utils.SplitText(sb.String(), "\n", false) {
		recMsg, err = recMsg.Reply(txt, &tele.ReplyOptions{HTMLParseMode: true})
		if err != nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func rollbackRss(msg *tele.Message, args string) error {
	helpMsg := "Rollback a rss feed!\n" +
		"simply pass the rss title with the following arguments:"
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	arg := argparser.ParseFlags(
		"rss",
		helpMsg,
		argList,
		argparser.CreateDBoolFlag("rollback", "r", "HEAD PARAM"),
		argparser.CreateDStringFlag("last-title", "t", "The last title to rollback to."),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(
			utils.CodeBlock(out, true),
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	title := strings.Join(arg.UnknownArgs, " ")
	title = strings.TrimSpace(title)
	if title == "" {
		_, _ = msg.ReplyText("Title is Required. Append --help for help")
		return nil
	}
	bot.RSSDictMutex.RLock()
	rssData, found := bot.RSSDict[title]
	bot.RSSDictMutex.RUnlock()
	if !found {
		_, _ = msg.ReplyText(
			"Title: " + title +
				" was not found in subscribed feeds.",
		)
		return nil
	}
	pargs := arg.Values
	lastTitle := pargs["last-title"].(string)

	bot.RSSDictMutex.Lock()
	rssData.LastFeed = ""
	rssData.LastTitle = lastTitle
	rssData.AllowSpam = true
	bot.RSSDictMutex.Unlock()

	_, _ = msg.Reply("Rollbacked Rss configuration successfully.",
		&tele.ReplyOptions{HTMLParseMode: true})
	db.SaveRSSDict(context.Background())

	logger.InfoW(
		"RSS Feed Rollbacked:",
		zap.Int64("By", msg.SenderID),
		zap.String("Title", title),
		zap.String("LastTitle", lastTitle),
	)
	return nil
}
