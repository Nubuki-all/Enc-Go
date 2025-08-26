package manage

import (
	"fmt"
	"strings"
	"strconv"

	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/entimer"
	"enc/internal/db"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/tele"
)

func ChangeEParams(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Changes bot encoding params;\n" +
		"\n" +
		"Requires full shell command with '''{input}''' for input and '''{output}''' output (use double quotes for better performance)\n" +
		"    - Binary name must me be included in argument and must installed locally or in docker\n" +
		"    - Both ffmpeg and handbrake-cli are tested and supported. (Test if others work)\n" +
		"    - Alternatively you can also create a custom bash script and pass the path of the script.\n" +
		"To set for FFMPEG2(or 3 or 4), add -2 or -3 and so on, before the shell command for encoding."
	if args == "" || utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	var alt, cmd string
	if strings.HasPrefix(args, "-2 ") {
		alt, cmd, _ = strings.Cut(args, " ")
		bot.FFmpeg2Args = cmd
	} else if strings.HasPrefix(args, "-3 ") {
		alt, cmd, _ = strings.Cut(args, " ")
		bot.FFmpeg3Args = cmd
	} else if strings.HasPrefix(args, "-4 ") {
		alt, cmd, _ = strings.Cut(args, " ")
		bot.FFmpeg4Args = cmd
	} else {
		cmd = args
		bot.FFmpegArgs = cmd
	}
	db.SaveFFmpegArgs(ctx)
	if alt != "" {
		utils.EJob.Reset(false)
	}
	_, _ = msg.Reply(
		fmt.Sprintf("<pre language=\"Changed encoding%s CLI parameters to:\">%s</pre>", alt, cmd),
		&tele.ReplyOptions{HtmlParseMode: true},
	)
	return nil
}

func CheckEParams(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	helpMsg := "Get custom encoding params.\n" +
		"Arguments:\n" +
		"    -2\n" +
		"    -3\n" +
		"    -4\n" +
		"    To check for ffmpeg2, 3 and 4 custom encoding params\n" +
		"No argument to check first encoding params"
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	needHelp := func() error {
		_, err := msg.ReplyText(fmt.Sprintf("Unknown argument: %s\nNeed help? Send `%s --help`.", args, cmd))
		return err
	}
	var arg, params string
	if args == "" {
		params = bot.FFmpegArgs
	} else if len(args) < 2 {
		return needHelp()
	} else {
		arg = args[:2]
		if arg  == "-2" {
		params = bot.FFmpeg2Args
		} else if arg  == "-3" {
		params = bot.FFmpeg3Args
		} else if arg  == "-4" {
		params = bot.FFmpeg4Args
		} else {
			return needHelp()
		}
	}
	if params == "" {
		idx := ""
		if arg != "" {
			idx = " " + arg
		}
		_, _ = msg.ReplyText(fmt.Sprintf("**Encoding params not found, use /set%s to set encoding params.**", idx))
		return nil
	}
	_, _ = msg.Reply(
		fmt.Sprintf("<pre language=\"Current ffmpeg%s CLI parameters:\">%s</pre>", arg, params),
		&tele.ReplyOptions{HtmlParseMode: true},
	)
	return nil
}

func ResetEParams(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	helpMsg := "Reset encoding params.\n" +
		"Behavior changes depending on whether its a suffixed encoding param or otherwise\n" +
		"Arguments:\n" +
		"    -2\n" +
		"    -3\n" +
		"    -4\n" +
		"    To initially delete, then reset suffixed encoding parameters; ffmpeg2, 3 and 4\n" +
		"No argument to immediately reset the first & default encoding parameters, it cannot be deleted."
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	needHelp := func() error {
		_, err := msg.ReplyText(fmt.Sprintf("Unknown argument: %s\nNeed help? Send `%s --help`.", args, cmd))
		return err
	}
	if args == "" {
		bot.FFmpegArgs = config.Conf.FFmpeg
	} else if len(args) < 2 {
		return needHelp()
	} else {
		arg := args[:2]
		if arg  == "-2" {
		return ResetSEParams(ctx, update, msg, arg)
		} else if arg  == "-3" {
		return ResetSEParams(ctx, update, msg, arg)
		} else if arg  == "-4" {
		return ResetSEParams(ctx, update, msg, arg)
		} else {
			return needHelp()
		}
	}
	_, _ = msg.Reply(
		fmt.Sprintf("<pre language=\"Reset encoding CLI parameters to:\">%s</pre>", config.Conf.FFmpeg),
		&tele.ReplyOptions{HtmlParseMode: true},
	)
	return nil
}

func ResetSEParams(ctx *ext.Context, update *ext.Update, msg *tele.Message, arg string) error {
	envNotSet := func() error {
		_, err := msg.ReplyText(fmt.Sprintf("Encoding param%s not set in .env or bot.", arg))
		return err
	}
	var defaultEnv string
	if arg  == "-2" {
		if bot.FFmpeg2Args != "" {
			bot.FFmpeg2Args = ""
		} else if defaultEnv = config.Conf.FFmpeg2; defaultEnv != "" {
			bot.FFmpeg2Args = defaultEnv
		} else {
			return envNotSet()
		}
	} else if arg  == "-3" {
		if bot.FFmpeg3Args != "" {
			bot.FFmpeg3Args = ""
		} else if defaultEnv = config.Conf.FFmpeg3; defaultEnv != "" {
			bot.FFmpeg3Args = defaultEnv
		} else {
			return envNotSet()
		}
	} else if arg  == "-4" {
		if bot.FFmpeg4Args != "" {
			bot.FFmpeg4Args = ""
		} else  if defaultEnv := config.Conf.FFmpeg4; defaultEnv != "" {
			bot.FFmpeg4Args = defaultEnv
		} else {
			return envNotSet()
		}
	}
	if defaultEnv == "" {
		_, _ = msg.ReplyText(fmt.Sprintf(
			"Encoding params%s deleted.\nTry again to reset to the param in .env",
			arg,
		))
	} else  {
		_, _ = msg.Reply(
			fmt.Sprintf("<pre language=\"Reset encoding CLI parameters%s to:\">%s</pre>", arg, defaultEnv),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
	}
	return nil
}

func SetMuxArgs(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Set, reset or disable muxing after transcoding.\n" +
		"Arguments:\n" +
		"    ffmpeg params without the (-i input & output)\n" +
        "        Do not pass encoding params, only map, metadata, dispositions are allowed.\n" +
        "    or:\n" +
        "\n" +
		"    reset\n" +
        "        to reset the mux_args to same parameter in env.\n" +
        "    clear\n" +
        "        to clear mux_args."
	if args == "" || utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	if strings.EqualFold(args, "clear") {
		if bot.MuxArgs == "" {
			_, _ = msg.ReplyText("**Muxing argument was not set; Therefore cannot clear!**")
			return nil
		}
		bot.MuxArgs = ""
		db.SaveMuxArgs(ctx)
		_, _ = msg.ReplyText("**Successfully unset mux_args**")
		return nil
	}
	if strings.EqualFold(args, "reset") {
		if config.Conf.MuxArgs == "" {
			_, _ = msg.ReplyText("**Muxing argument was not set in .env; Therefore cannot reset!**")
			return nil
		}
		args = config.Conf.MuxArgs
	} 
	bot.MuxArgs = args
	db.SaveMuxArgs(ctx)
	_, _ = msg.Reply(
		fmt.Sprintf("<pre language=\"Changed mux_args to:\">%s</pre>", args),
		&tele.ReplyOptions{HtmlParseMode: true},
	)
	return nil
}

func GetMuxArgs(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Get currently set mux_args.\n" +
		"Requires no arguments and any given will be ignored."
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	params := bot.MuxArgs
	if params == "" {
		_, _ = msg.ReplyText("__mux_args not set.__")
		return nil
	}
	_, _ = msg.Reply(
		fmt.Sprintf("<pre language=\"Current mux_args:\">%s</pre>", params),
		&tele.ReplyOptions{HtmlParseMode: true},
	)
	return nil
}


func Version(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Tag a realese with what numbers you specify:\n" +
	"    - numbers (floats can be specified.)\n" +
	"    - off/disable to turn off\n" +
	"Sending this command without argument will check for previous version tags and display them."
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	notSet := func() {
		_, _ = msg.ReplyText("__Not Set!__")
	}
	vTag := bot.ReleaseVersion
	if args == "" {
		if vTag == "" {
			notSet()
		} else {
			_, _ = msg.ReplyText(fmt.Sprintf("**Current version tag:** `V%s`", vTag))
		}
		return nil
	}
	if strings.EqualFold(args, "disable") || strings.EqualFold(args, "off") {
		if vTag == "" {
			notSet()
		} else {
			bot.ReleaseVersion = ""
			_, _ = msg.ReplyText(fmt.Sprintf("**Removed V%s tag Successfully!**", vTag))
		}
		return nil
	}
	if _, err := strconv.ParseFloat(args, 64); err != nil {
		_, _ = msg.ReplyText(fmt.Sprintf("Unknown argument: `%s`", args))
		return nil
	}
	bot.ReleaseVersion = args
	_, _ = msg.ReplyText(
		fmt.Sprintf("**Added v%s tag successfully!**", args),
	)
	return nil
}


func MGlobalPOptions(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
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
	helpMsg := "Disable/enable a parsing mechanism:\nRequired arguments:"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDBoolFlag(
			"enable-anilist",
			"",
			"Enables anilist parsing.",
		),
		argparser.CreateDBoolFlag(
			"disable-anilist",
			"",
			"Disables anilist parsing.",
		),
		argparser.CreateDBoolFlag(
			"enable-captions",
			"",
			"Treats file captions as actual filename unless newlines are detected.",
		),
		argparser.CreateDBoolFlag(
			"disable-captions",
			"",
			"Filenames are gotten from files.",
		),
		argparser.CreateDBoolFlag(
			"check",
			"c",
			"Check the current parsing options.",
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
	check := pargs["check"].(bool)
	if check {
		_, _ = msg.ReplyText(fmt.Sprintf(
			"**AniList is enabled:** `%s`\n**Caption parsing is enabled:** `%s`",
			utils.YesNo(bot.AnilistParse),
			utils.YesNo(bot.CaptionParse),
		))
		return nil
	}
	dAnilist := pargs["disable-anilist"].(bool)
	eAnilist := pargs["enable-anilist"].(bool)
	dCaptions := pargs["disable-captions"].(bool)
	eCaptions := pargs["enable-captions"].(bool)
	var reply string
	anilist := bot.AnilistParse
	captions := bot.CaptionParse
	if dAnilist {
		if !anilist {
			reply = "__Anilist is not enabled.__"
		} else {
			bot.AnilistParse = false
			reply = "**Disabled anilist parsing successfully**"
		}
	} else if eAnilist {
		if anilist {
			reply = "__Anilist is already enabled.__"
		} else {
			bot.AnilistParse = true
			reply = "**Enabled anilist parsing successfully**"
		}
	} else if dCaptions {
		if !captions {
			reply = "__Caption parsing is not enabled.__"
		} else {
			bot.CaptionParse = false
			reply = "**Disabled caption parsing successfully**"
		}
	} else if eCaptions {
		if captions {
			reply = "__Caption parsing is already enabled.__"
		} else {
			bot.CaptionParse = true
			reply = "**Enabled caption parsing successfully**"
		}
	} else {
		_, _ = msg.Reply("Invaid arguments."+
			"\nNeed help? Send <code>"+
			cmd+
			" --help</code>.",
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	_, _ = msg.ReplyText(reply)
	return nil
}

func Pause(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Pause bot;\n" +
	"Prevent bot from encoding for a specified duration.\n" +
	"muxing and renaming are not paused.\n" +
	"Arguments:\n" +
	"    - A number\n" +
	"        • for instance: /pause 900 to pause for 900 seconds or;\n" +
	"          /pause 0 to pause indefinitely till you cancel with /pause off\n" +
	"    - off/disable (To unpause)\n" +
	"    - no argument; (To check state)"
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	var reply string

	if args == "" {
		status := "not"
		if b, _ := utils.GetPauseStatus(0); !b {
			status = "currently"
		}
		reply = fmt.Sprintf("**Pause Status:** `Bot is %s paused.`", status)
	} else	if strings.EqualFold(args, "off") || strings.EqualFold(args, "disable") {
		if b, _ := utils.GetPauseStatus(0); !b {
			reply = "**Bot was not paused**"
		} else {
			entimer.Timer.StopTimer()
			reply = "**Pausing Cancelled**"
		}
	} else if !utils.IsPositiveNumber(args) {
		_, _ = msg.Reply("Invaid Number."+
			"\nNeed help? Send <code>"+
			update.Args()[0]+
			" --help</code>.",
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	if reply != "" {
		_, _ = msg.ReplyText(reply)
		return nil
	}
	i, err := strconv.Atoi(args)
	if i == 0 {
		reply = "**Bot has been paused indefinitely.**"
	} else {
		reply = fmt.Sprintf("`Bot has been paused for %s`", utils.TimeFormatter(float64(i)))
	}
	nMsg, err := msg.ReplyText(reply)
	if err != nil {
		return err
	}
	mList := tele.BcMsg(reply, msg.SenderID, []*tele.Message{nMsg})
	if i > 0 {
		entimer.Timer.NewTimer(i, mList)
	} else {
		entimer.Timer.PauseIndefinitely(mList)
	}
	return nil
}