package manage

import (
	"fmt"
	//"strconv"
	"strings"

	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
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
		"Or send one of the following params:\n"
	if args == "" {
		args += "--help"
	}
	if strings.HasPrefix(args, "-") {
		argList, err := utils.SplitQuotedFieldsErr(args)
		if err != nil {
			msg.ReplyText(err.Error())
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
		)
		if out := arg.Output; out != "" {
			_, _ = msg.ReplyText(utils.CodeBlock(out))
			_, _ = msg.ReplyText(out)
			fmt.Println(out)
			return nil
		} else if err := arg.Err; err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
		pargs := arg.Values
		if pargs["status"].(bool) {
			status := "Custom Rename format is"
			if bot.CustomRename != "" {
				status += "set."
			} else {
				status += "not set."
			}
			_, _ = msg.ReplyText(status)
		} else if pargs["reset"].(bool) {
			bot.CustomRename = config.Conf.CustomRename
			db.SaveFilterSettings(ctx)
			_, _ = msg.ReplyText("`Reseted custom rename format successfully!`")
		} else if pargs["delete"].(bool) {
			bot.CustomRename = ""
			db.SaveFilterSettings(ctx)
			_, _ = msg.ReplyText("`Deleted custom rename format successfully!`")
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
