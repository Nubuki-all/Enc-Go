package manage

import (
	"fmt"
	"strconv"
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
		"Flags:",
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
				cmd + " --help` for help.",
		)
		return nil
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

func VAutoRename(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	// cmd := update.Args()[0]
	helpMsg := "A command to view Rename Filter Settings.\nFlags:" +
		"\n    --list/-l : List all Rename filters" +
		"\n    Name/id of rename filter gotten from listing to view filter parameters."
	if args == "" || strings.EqualFold(args, "-h") || strings.EqualFold(args, "--help") {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	if strings.EqualFold(args, "-l") || strings.EqualFold(args, "--list") {
		var sb strings.Builder
		for i, filter := range bot.RenameFilters {
			sb.WriteString(fmt.Sprintf("%v. `%s`\n", i, filter.Match))
		}
		_, _ = msg.ReplyText(sb.String())
		return nil
	}
	var rFilter *types.RenameFilterData
	var id int
	if i, err := strconv.Atoi(args); err == nil {
		id = i
	}
	for i, filter := range bot.RenameFilters {
		if id == i || args == filter.Match {
			rFilter = filter
			break
		}
	}
	if rFilter == nil {
		_, _ = msg.ReplyText("**Rename Filter not found.**")
		return nil
	}
	var sb strings.Builder
	sb.WriteString(
		fmt.Sprintf("**Match:**\n`%s`\n\n", rFilter.Match),
	)
	sb.WriteString(
		fmt.Sprintf("**Rename to:**\n`%s`\n\n", rFilter.ReplaceName),
	)
	caption := rFilter.ReplaceCaption
	if caption == "" {
		caption = rFilter.ReplaceName
	}
	sb.WriteString(
		fmt.Sprintf("**Rename caption to:**\n`%s`\n\n", caption),
	)
	if rFilter.ReplaceBanner != "" {
		sb.WriteString(
			fmt.Sprintf("**Rename Banner to:**\n`%s`\n\n", rFilter.ReplaceBanner),
		)
	}
	yStr := "No."
	if rFilter.CaseSensitive {
		yStr = "Yes."
	}
	sb.WriteString(
		fmt.Sprintf("**Case Sensitive:**  `%s`\n\n", yStr),
	)
	if rFilter.DisableAnilist {
		yStr = "Yes."
	} else {
		yStr = "No."
	}
	sb.WriteString(
		fmt.Sprintf("**Disable Anilist:**  `%s`\n\n", yStr),
	)
	_, _ = msg.ReplyText(sb.String())
	return nil
}

func DelAutoRename(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := utils.StringFormat(
		"Deletes previously saved rename filters!",
		"Requires the match name (case sensitive) or id, ",
		"to get both list the saved rename filters.",
		"Please use double qoutes while passing arguments ",
		"with spaces to avoid unknown arguments.",
		"Flags:",
	)
	if args == "" {
		args += "--help"
	}
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"name",
			"n",
			"Match name of rename filter.",
		),
		argparser.CreateDStringFlag(
			"id",
			"i",
			"Id of rename filter",
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
	match := pargs["name"].(string)
	id := pargs["id"].(string)
	if match == "" && id == "" {
		_, _ = msg.ReplyText(
			fmt.Sprintf("Missing required argument.\nNeed help? Send: `%s --help`", cmd),
		)
		return nil
	}
	iD := -1
	if match != "" {
		for i, filter := range bot.RenameFilters {
			if match == filter.Match {
				iD = i
				break
			}
		}
		if iD == -1 {
			_, _ = msg.ReplyText(
				fmt.Sprintf("Could not find `%s` in rename filters", match),
			)
			return nil
		}
	} else {
		iD, err := strconv.Atoi(id)
		if err != nil {
			_, _ = msg.ReplyText(
				fmt.Sprintf("Invalid number: `%s`", id),
			)
			return nil
		}
		if iD > (len(bot.RenameFilters) - 1) {
			_, _ = msg.ReplyText(
				fmt.Sprintf("Supplied number: `%s` is not a valid id.", id),
			)
			return nil
		}
	}
	bot.RenameFilters = utils.RemoveRFilterByIndex(bot.RenameFilters, iD)
	db.SaveRenameFilters(ctx)
	_, _ = msg.ReplyText("**Removed filter from Rename Filters Successfully**")
	return nil
}
