// Package info contains handlers that make info queries
package info

import (
	"strings"

	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/parser"
	"enc/utils/tele"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func Anime(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "Fetch anime info from Anilist\n" +
		"Arguments:\n"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"query",
			"q",
			"Title/ID of anime  (can be given without this flag.)",
		),
		argparser.CreateDBoolFlag(
			"mal",
			"m",
			"Searching with a mal id",
		),
		argparser.CreateDBoolFlag(
			"is-id",
			"i",
			"query is an id.",
		),
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
	pargs := arg.Values
	query := pargs["query"].(string)
	if query == "" {
		query = strings.Join(arg.UnknownArgs, " ")
	}
	if query == "" {
		_, _ = msg.Reply(
			"<b>Please supply a query to search for or resend the command without arguments for help.</b>",
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	}
	mal := pargs["mal"].(bool)
	isID := pargs["is-id"].(bool)
	cover, txt, err := parser.AnimeArch(query, mal, isID)
	if err != nil {
		_, _ = msg.Reply("An Error occured:\n<code>"+err.Error()+"</code",
			&tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	_, _ = msg.ReplyMediaDoc(
		&tg.InputMediaPhotoExternal{URL: cover},
		&tele.UploadOptions{Caption: txt, HTMLParseMode: true},
	)
	return nil
}

func AnimeAiring(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "Fetch anime info from Anilist\n" +
		"Arguments:\n"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"query",
			"q",
			"Title/ID of anime  (can be given without this flag.)",
		),
		argparser.CreateDBoolFlag(
			"mal",
			"m",
			"Searching with a mal id",
		),
		argparser.CreateDBoolFlag(
			"is-id",
			"i",
			"query is an id.",
		),
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
	pargs := arg.Values
	query := pargs["query"].(string)
	if query == "" {
		query = strings.Join(arg.UnknownArgs, " ")
	}
	if query == "" {
		_, _ = msg.Reply(
			"<b>Please supply a query to search for or resend the command without arguments for help.</b>",
			&tele.ReplyOptions{HTMLParseMode: true},
		)
		return nil
	}
	mal := pargs["mal"].(bool)
	isID := pargs["is-id"].(bool)
	cover, txt, err := parser.AiringAnim(query, mal, isID)
	if err != nil {
		_, _ = msg.Reply("An Error occured:\n<code>"+err.Error()+"</code",
			&tele.ReplyOptions{HTMLParseMode: true})
		return nil
	}
	_, _ = msg.ReplyMediaDoc(
		&tg.InputMediaPhotoExternal{URL: cover},
		&tele.UploadOptions{Caption: txt, HTMLParseMode: true},
	)
	return nil
}
