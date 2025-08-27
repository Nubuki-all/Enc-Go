package manage

import (
	"fmt"
	"slices"
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
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
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
			_, _ = msg.Reply(utils.CodeBlock(out, true),
				&tele.ReplyOptions{HtmlParseMode: true},
			)
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
			_, _ = msg.Reply(
				"Kindly send "+
					cmd+
					" --help",
				&tele.ReplyOptions{HtmlParseMode: true},
			)
		}
		return nil

	}
	bot.CustomRename = strings.Trim(args, `"`)
	db.SaveFilterSettings(ctx)
	_, _ = msg.ReplyText("**Custom rename format has been changed.**")
	return nil
}

func AutoRename(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if args == "" {
		args += "--help"
	}
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
		"    00 - Disable all sort of parsing and use the original title instead.",
		"Flags:",
	)
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
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	match := pargs["match"].(string)
	newName := pargs["rename"].(string)
	if !(match != "" && newName != "") {
		_, _ = msg.Reply(
			"Args;\n"+
				"--rename & --match"+
				"\nare Required. Do <code>"+
				cmd+" --help</code> for help.",
			&tele.ReplyOptions{HtmlParseMode: true},
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

func VAutoRename(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "A command to view Rename Filter Settings.\nFlags:" +
		"\n    --list/-l : List all Rename filters" +
		"\n    Name/id of rename filter gotten from listing to view filter parameters."
	if args == "" || strings.EqualFold(args, "-h") || strings.EqualFold(args, "--help") {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}
	if strings.EqualFold(args, "-l") || strings.EqualFold(args, "--list") {
		if len(bot.RenameFilters) == 0 {
			_, _ = msg.ReplyText("`No rename filters yet!`")
			return nil
		}
		var sb strings.Builder
		for i, filter := range bot.RenameFilters {
			sb.WriteString(fmt.Sprintf("%v. `%s`\n", i, filter.Match))
		}
		_, _ = msg.ReplyText(sb.String())
		return nil
	}
	var rFilter *types.RenameFilterData
	id := 1
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
		fmt.Sprintf("**Disable Anilist:**  `%s`", yStr),
	)
	_, _ = msg.ReplyText(sb.String())
	return nil
}

func DelAutoRename(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if args == "" {
		args += "--help"
	}
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
			"ID of rename filter",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	match := pargs["name"].(string)
	id := pargs["id"].(string)
	if match == "" && id == "" {
		_, _ = msg.Reply(
			fmt.Sprintf("Missing required argument.\nNeed help? Send: <code>%s --help</code>", cmd),
			&tele.ReplyOptions{HtmlParseMode: true},
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
		ID, err := strconv.Atoi(id)
		if err != nil {
			_, _ = msg.ReplyText(
				fmt.Sprintf("Invalid number: `%s`", id),
			)
			return nil
		}
		if ID > (len(bot.RenameFilters) - 1) {
			_, _ = msg.ReplyText(
				fmt.Sprintf("Supplied number: `%s` is not a valid id.", id),
			)
			return nil
		}
		iD = ID
	}
	bot.RenameFilters = slices.Delete(bot.RenameFilters, iD, iD+1)
	db.SaveRenameFilters(ctx)
	_, _ = msg.ReplyText("**Removed filter from Rename Filters Successfully**")
	return nil
}

func Filter(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if args == "" {
		args += "--help"
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := utils.StringFormat(
		"Another filter command. (Not to be confused with namefilter)",
		"Unlike (re)name filter this targets the file_name and is universal.",
		"Both file tag & caption tag accepts disable as an argument ",
		"which disables the tag",
		"Please use double qoutes while passing arguments ",
		"to avoid malformed filters",
		"Requires the following arguments:",
	)
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"remove",
			"r",
			"List of strings to remove, delimited by '|'",
		),
		argparser.CreateDStringFlag(
			"tag-file",
			"f",
			"Tag all files as [args]",
		),
		argparser.CreateDStringFlag(
			"tag-caption",
			"c",
			"Tag all captions as … "+
				"to add to the generated tags instead of replacing, pass \"{auto} my_tags\" as argument",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	remove := pargs["remove"].(string)
	tagFile := pargs["tag-file"].(string)
	tagCap := pargs["tag-caption"].(string)
	if remove == "" && tagFile == "" && tagCap == "" {
		_, _ = msg.Reply(
			fmt.Sprintf("Missing required argument.\nNeed help? Send: <code>%s --help</code>", cmd),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	bot.Filters = &types.FilterData{
		Fil1: remove,
		Fil2: tagFile,
		Fil3: tagCap,
	}
	db.SaveFilterData(ctx)
	_, _ = msg.ReplyText("**Updated Filter Successfully**")
	return nil
}

func VFilter(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "View currently added filter.\nRequires no arguments."
	if strings.EqualFold(args, "-h") || strings.EqualFold(args, "--help") {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}
	if bot.Filters == nil {
		_, _ = msg.ReplyText("**Filter is not set.**")
		return nil
	}
	filter := bot.Filters
	filterStr := fmt.Sprintf("Bot Will Automatically:\n\n"+
		"**Remove:-** `%s`\n"+
		"**Tag Files as:-** `%s`\n"+
		"**Tag caption as:-** `%s`",
		filter.Fil1,
		filter.Fil2,
		filter.Fil3,
	)
	_, _ = msg.ReplyText(filterStr)
	return nil
}

func DelFilter(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Delete previously added filter.\nRequires no arguments."
	if strings.EqualFold(args, "-h") || strings.EqualFold(args, "--help") {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}
	if bot.Filters == nil {
		_, _ = msg.ReplyText("**Filter is not set.**")
		return nil
	}
	bot.Filters = nil
	db.SaveFilterData(ctx)
	_, _ = msg.ReplyText("`Filters Deleted!`")
	return nil
}

func MatchFilter(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if args == "" {
		args += "--help"
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := utils.StringFormat(
		"A command to auto tag files if they contain specified strings.",
		"Note: It matches the entire file name.",
		"Please use double qoutes while passing arguments ",
		"to avoid malformed filters",
		"Flags:",
	)
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"tag-file",
			"f",
			"Tag for file without brackets",
		),
		argparser.CreateDStringFlag(
			"tag-caption",
			"c",
			"Tag for caption, also shares the same behavior as the global filter.",
		),
		argparser.CreateDStringFlag(
			"match",
			"m",
			"[Required] String to match.",
		),
		argparser.CreateDBoolFlag(
			"match-once",
			"o",
			"Only use this for matched file instead of the usual aggregate behavior.",
		),
		argparser.CreateDBoolFlag(
			"case-sensitive",
			"s",
			"Enable case sensitive matching.",
		),
		argparser.CreateDBoolFlag(
			"release",
			"r",
			"Whether to only match release group names, would only match once.",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	match := pargs["match"].(string)
	if match == "" {
		_, _ = msg.Reply(
			"Argument;\n"+
				"--match"+
				"\nis required. Do <code>"+
				cmd+" --help</code> for help.",
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	tagFile := pargs["tag-file"].(string)
	tagCaption := pargs["tag-caption"].(string)
	mOnce := pargs["match-once"].(bool)
	cSensitive := pargs["case-sensitive"].(bool)
	release := pargs["release"].(bool)
	var filters []*types.MatchedFilterData
	if release {
		filters = bot.ReleaseFilters
	} else {
		filters = bot.NameFilters
	}
	if MatchFilterExists(match, filters) || (release && ReleaseFilterExists(match, cSensitive)) {
		_, _ = msg.ReplyText("File/Release Filter with given matched name already" +
			" exists. Note: You can only mix case-sensitive and non case-sensitive filters if not matching release group names.",
		)
		return nil
	}
	filter := &types.MatchedFilterData{
		Match:         match,
		FileTag:       tagFile,
		CaptionTag:    tagCaption,
		CaseSensitive: cSensitive,
		MatchOnce:     mOnce,
	}
	filters = append(filters, filter)
	updateMatchFilter(filters, release)
	db.SaveMatchedFilters(ctx)
	_, _ = msg.ReplyText("**Added new Name filter Successfully**")
	return nil
}

func ManageMatchFilter(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if args == "" {
		args += "--help"
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "A command to view or delete Filename/Release Filter Settings.\nFlags:"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"id",
			"i",
			"Id of filter to view.",
		),
		argparser.CreateDStringFlag(
			"name",
			"n",
			"Name of filter to view.",
		),
		argparser.CreateDBoolFlag(
			"list",
			"l",
			"List all name/release filters.",
		),
		argparser.CreateDBoolFlag(
			"delete",
			"d",
			"Whether to delete filter. If not specified view instead.",
		),
		argparser.CreateDBoolFlag(
			"release",
			"r",
			"Whether to view/delete release group filters instead.",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	list := pargs["list"].(bool)
	release := pargs["release"].(bool)
	Delete := pargs["delete"].(bool)
	var filters []*types.MatchedFilterData
	var typeName string
	if release {
		filters = bot.ReleaseFilters
		typeName = "Release Name"
	} else {
		filters = bot.NameFilters
		typeName = "Filename"
	}
	if list {
		if len(filters) == 0 {
			_, _ = msg.ReplyText("`No name/release filters yet!`")
			return nil
			listHeader := "**List of all "
			listHeader += typeName
			listHeader += " filters**\n"
			var sb strings.Builder
			for i, filter := range filters {
				sb.WriteString(fmt.Sprintf("%v. `%s`\n", i, filter.Match))
			}
			_, _ = msg.ReplyText(listHeader + sb.String())
			return nil
		}
	}
	name := pargs["name"].(string)
	iD := pargs["id"].(string)
	if name == "" && iD == "" {
		_, _ = msg.Reply(
			"<code>At least one argument excluding --release & --delete is required.</code>",
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	var Filter *types.MatchedFilterData
	id := -1
	i, err := strconv.Atoi(iD)
	if err == nil {
		id = i
	} else {
		_, _ = msg.ReplyText(
			fmt.Sprintf("**--id:** `%s` is not a valid number.", iD),
		)
		return nil
	}
	for i, filter := range filters {
		if id == i || name == filter.Match {
			Filter = filter
			id = i
			break
		}
	}
	if Filter == nil {
		_, _ = msg.ReplyText(
			"**" +
				typeName +
				" Filter not found.**",
		)
		return nil
	}
	if Delete {
		filters = slices.Delete(filters, id, id+1)
		updateMatchFilter(filters, release)
		db.SaveMatchedFilters(ctx)
		_, _ = msg.ReplyText(
			"**Deleted " +
				typeName +
				" filter Successfully**",
		)
		return nil
	}
	var sb strings.Builder
	sb.WriteString(
		fmt.Sprintf("**Match:**\n`%s`\n\n", Filter.Match),
	)
	if fTag := Filter.FileTag; fTag != "" {
		sb.WriteString(
			fmt.Sprintf("**Tag files as:**\n`%s`\n\n", fTag),
		)
	}
	if cTag := Filter.CaptionTag; cTag != "" {
		sb.WriteString(
			fmt.Sprintf("**Tag caption to:**\n`%s`\n\n", cTag),
		)
	}

	yStr := "No."
	if Filter.CaseSensitive {
		yStr = "Yes."
	}
	sb.WriteString(
		fmt.Sprintf("**Case Sensitive:**  `%s`\n\n", yStr),
	)
	if !release {
		if Filter.MatchOnce {
			yStr = "Yes."
		} else {
			yStr = "No."
		}

		sb.WriteString(
			fmt.Sprintf("**Match Once:**  `%s`\n\n", yStr),
		)
	}
	sb.WriteString("**Empty fields are not displayed.**")
	_, _ = msg.ReplyText(sb.String())
	return nil
}

func ExtCapFilter(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if args == "" {
		args += "--help"
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "A command to auto tag captions if release group is matched and file contains futher matches.\n" +
		"Note: It matches the release group & file name.\n" +
		"Please use double qoutes while passing arguments " +
		"to avoid malformed filters.\n" +
		"Flags:\n"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"release-group",
			"r",
			"Release group name to match.",
		),
		argparser.CreateDStringSliceFlag(
			"filter",
			"f",
			"Format: 'item to match in file name||caption tag||case-sensitive?||match once? (bool)', flag can be used multiple times to match different tags in file name.",
		),

		argparser.CreateDBoolFlag(
			"case-sensitive",
			"c",
			"if matching release group should be case sensitive.",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	release := pargs["release-group"].(string)
	data := pargs["filter"].([]string)
	if release == "" || len(data) == 0 {
		_, _ = msg.Reply(
			"Arguments;\n"+
				"--release-group & --filter"+
				"\nare required. Do <code>"+
				cmd+" --help</code> for help.",
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	cSensitive := pargs["case-sensitive"].(bool)
	if ReleaseExtFilterExists(release, cSensitive) {
		_, _ = msg.ReplyText("Release Ext Filter with given matched name already" +
			" exists. Note: You cannot mix case-sensitive and non case-sensitive filters.",
		)
		return nil
	}
	datas := convertStrToMtchFiltr(data)
	if len(datas) == 0 {
		_, _ = msg.Reply("Valid filter data could not be constructed from arguments."+
			"\nNeed help? Send <code>"+
			cmd+
			" --help</code>.",
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	filter := &types.RGExtendedFilterData{
		Match:         release,
		Data:          datas,
		CaseSensitive: cSensitive,
	}
	bot.RGExtendedFilters = append(bot.RGExtendedFilters, filter)
	db.SaveRGExtendedFilters(ctx)
	_, _ = msg.ReplyText("**Added a new extended release filter Successfully**")
	return nil
}

func ManageExtCapFilter(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	if args == "" {
		args += "--help"
	}
	cmd := update.Args()[0]
	argList, err := utils.SplitQuotedFieldsErr(args)
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	helpMsg := "A command to view or delete extended release filter settings.\nFlags:"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDStringFlag(
			"id",
			"i",
			"Id of filter to view.",
		),
		argparser.CreateDStringFlag(
			"name",
			"n",
			"Name of filter to view.",
		),
		argparser.CreateDBoolFlag(
			"list",
			"l",
			"List all name/release filters.",
		),
		argparser.CreateDBoolFlag(
			"delete",
			"d",
			"Whether to delete filter. If not specified view instead.",
		),
	)
	if out := arg.Output; out != "" {
		_, _ = msg.Reply(utils.CodeBlock(out, true),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	} else if err := arg.Err; err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	pargs := arg.Values
	list := pargs["list"].(bool)
	Delete := pargs["delete"].(bool)
	if list {
		if len(bot.RGExtendedFilters) == 0 {
			_, _ = msg.ReplyText("`No extended release filters yet!`")
			return nil
			listHeader := "**List of all extended release filters**\n"
			var sb strings.Builder
			for i, filter := range bot.RGExtendedFilters {
				sb.WriteString(fmt.Sprintf("%v. `%s`\n", i, filter.Match))
			}
			_, _ = msg.ReplyText(listHeader + sb.String())
			return nil
		}
	}
	name := pargs["name"].(string)
	iD := pargs["id"].(string)
	if name == "" && iD == "" {
		_, _ = msg.ReplyText("`At least one additional argument is required if --list is not specified.`")
		return nil
	}
	var Filter *types.RGExtendedFilterData
	id := -1
	i, err := strconv.Atoi(iD)
	if err == nil {
		id = i
	} else {
		_, _ = msg.ReplyText(
			fmt.Sprintf("**--id:** `%s` is not a valid number.", iD),
		)
		return nil
	}
	for i, filter := range bot.RGExtendedFilters {
		if id == i || name == filter.Match {
			Filter = filter
			id = i
			break
		}
	}
	if Filter == nil {
		_, _ = msg.ReplyText(
			"** Extended release filter not found.**",
		)
		return nil
	}
	if Delete {
		bot.RGExtendedFilters = slices.Delete(bot.RGExtendedFilters, id, id+1)
		db.SaveRGExtendedFilters(ctx)
		_, _ = msg.ReplyText(
			"**Deleted extended release filter Successfully**",
		)
		return nil
	}
	var sb strings.Builder
	sb.WriteString(
		fmt.Sprintf("**Release Group:** `%s`\n", Filter.Match),
	)
	yStr := "No."
	if Filter.CaseSensitive {
		yStr = "Yes."
	}
	sb.WriteString(
		fmt.Sprintf("**Case Sensitive:** `%s`\n\n", yStr),
	)
	for i, data := range Filter.Data {
		sb.WriteString(fmt.Sprintf("%s. ", i))
		sb.WriteString(
			fmt.Sprintf("**Match:** `%s`\n", data.Match),
		)
		sb.WriteString(
			fmt.Sprintf("**Tag as:** `%s`\n", data.CaptionTag),
		)
		if data.CaseSensitive {
			sb.WriteString(
				"**Case Sensitive:**  `Yes`\n",
			)
		}
		if data.MatchOnce {
			sb.WriteString(
				"**Match Once:**  `Yes`\n",
			)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("**Empty fields are not displayed.**")
	_, _ = msg.ReplyText(sb.String())
	return nil
}
