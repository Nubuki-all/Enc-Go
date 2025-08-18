package manage

import (
	//"fmt"
	//"strconv"
	"strings"

	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/tele"
)

func CustomRename(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	cmd := update.Args()[0]
	helpMsg := "Set a custom rename format if args are passed without parameters\n" +
		"Available variables:\n" +
		"    {season} Season number ('S' not added)\n" +
		"    {episode} Episode number\n" +
		"    {title} Title of video\n" +
		"    {quality} Quality of anime (Already enclosed in [])\n" +
		"    {codec} codec of video (Already enclosed in [])\n" +
		"    {audio} Audio(s) in video\n" +
		"    *Extension is added automatically\n" +
		"Or send one of the following params:"
	if args == "" {
		args += "--help"
	}
	notSet := func() error {
		_, err := msg.ReplyText("`Custom rename format not set!`")
		return err
	}
	if strings.HasPrefix(args, "-") {
		argList, err := utils.SplitQuotedFieldsErr(args)
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
		arg := argparser.ParseFlags(
			cmd,
			helpMsg,
			argList,
			argparser.CreateDBoolFlag(
				"status",
				"s",
				"Check if a Custom rename format is set.",
			),
			argparser.CreateDBoolFlag(
				"reset",
				"r",
				"Reset Custom rename format to format given in env.",
			),
			argparser.CreateDBoolFlag(
				"delete",
				"d",
				"Delete/unsets Custom rename format.",
			),
			argparser.CreateDBoolFlag(
				"check",
				"c",
				"Check Custom rename format.",
			),
		)
		if out := arg.Output; out != "" {
			_, _ = msg.ReplyText(utils.CodeBlock(out))
			return nil
		} else if err := arg.Err; err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
		pargs := arg.Values
		if pargs["status"].(bool) {
			status := "`Custom Rename format is "
			if bot.CustomRename == "" {
				status += "not "
			}
			status += "set.`"
			_, _ = msg.ReplyText(status)
		} else if pargs["reset"].(bool) {
			bot.CustomRename = config.Conf.CustomRename
			db.SaveFilterSettings(ctx)
			_, _ = msg.ReplyText("`Reseted custom rename format successfully!`")
		} else if pargs["delete"].(bool) {
			if bot.CustomRename == "" {
				return notSet()
			}
			bot.CustomRename = ""
			db.SaveFilterSettings(ctx)
			_, _ = msg.ReplyText("`Deleted custom rename format successfully!`")
		} else if pargs["check"].(bool) {
			if bot.CustomRename == "" {
				return notSet()
			}
			_, _ = msg.ReplyText(
				"`" + bot.CustomRename + "`",
			)
		} else {
			_, _ = msg.ReplyText("Kindly send " + cmd + " --help")
		}
		return nil

	}
	bot.CustomRename = strings.Trim(args, `"`)
	db.SaveFilterSettings(ctx)
	_, _ = msg.ReplyText("**Custom rename format has been changed.**")
	return nil
}

func AutoRename(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := utils.StringFormat(
		"A command to enable Auto renaming for matched files.",
		"Note: It matches Title not file name, so you might need ",
		"to check the title first before using.",
		"Please use double qoutes while passing arguments ",
		"to avoid malformed filters",
		"Each replace argument (All string arguments except match) accepts;",
		"    00 - Disable all sort of parsing and use the original title instead. ",
	)
	if args == "" {
		args += "--help"
	}
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"rename",
			"r",
			"[Required] New Title to rename to.",
		),
		argparser.CreateDStringFlag(
			"caption",
			"c",
			"New Title for caption, if 0 is passed, "+
				"Filter is disabled for caption, if 1 or nothing is passed "+
				"New title is taken from rename argument",
		),
		argparser.CreateDStringFlag(
			"banner",
			"b",
			"New Title for banners. If anilist is not disabled, the given "+
				"title is parsed by anilist. Pass 1 or 0 to re-use the caption or rename arguments respectively.",
		),
		argparser.CreateDStringFlag(
			"match",
			"m",
			"[Required] Original Title to match.",
		),
		argparser.CreateDBoolFlag(
			"disable-anilist",
			"d",
			"Disable parsing with Anilist.",
		),
		argparser.CreateDBoolFlag(
			"case-sensitive",
			"s",
			"Enable case sensitive parsing.",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.ReplyText(utils.CodeBlock(out))
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	match := pargs["match"].(string)
	newName := pargs["rename"].(string)
	if !(match != "" && newName != "") {
		_, _ = msg.ReplyText(
			"Args;\n" +
				"--rename & --match" +
				"\nare Required. Do `" +
				cmd + "--help` for help.",
		)
	}
	caption := pargs["caption"].(string)
	banner := pargs["banner"].(string)
	dAni := pargs["disable-anilist"].(bool)
	cSensitive := pargs["case-sensitive"].(bool)
	if RenameFilterExists(match, cSensitive) {
		_, _ = msg.ReplyText("Rename Filter with given matched name already" +
			" exists. Note: You can not mix case-sensitive and non case-sensitive filters.",
		)
		return nil
	}
	arFilter := &types.RenameFilterData{
		Match:          match,
		ReplaceName:    newName,
		ReplaceCaption: caption,
		ReplaceBanner:  banner,
		CaseSensitive:  cSensitive,
		DisableAnilist: dAni,
	}
	bot.RenameFilters = append(bot.RenameFilters, arFilter)
	db.SaveRenameFilters(ctx)
	_, _ = msg.ReplyText("**Added new Rename Filter Successfully**")
	return nil
}

func RenameFilterExists(name string, caseSensitive bool) bool {
	for _, filter := range bot.RenameFilters {
		if strings.EqualFold(filter.Match, name) {
			if !(caseSensitive || filter.CaseSensitive) {
				return true
			}
			if filter.Match == name {
				return true
			}
		}
	}
	return false
}