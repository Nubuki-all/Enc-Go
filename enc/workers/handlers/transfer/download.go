package transfer

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/tg"

	// "enc/internal/bot"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func Download(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	var err error
	helpMsg := "Downloads the replied media: to a location (specified) locally\n" +
		"Available arguments:\n" +
		"  Path to download (Non-quoted). End the args with '/' to specify the folder in which to download and let the bot auto detected filename\n" +
		"  or flags (path is ignored):"
	helpFunc := func(s string) error {
		_, err := msg.ReplyText(
			s + "\nNeed help? Send `" +
				cmd + " --help`",
		)
		return err
	}
	message := update.EffectiveMessage
	if msg.ReplyToID != 0 {
		err = message.SetRepliedToMessage(ctx, ctx.Raw, ctx.PeerStorage)
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
	} else {
		return helpFunc("Reply the media message to download.")
	}
	repMsg := message.ReplyToMessage
	if repMsg.Media == nil {
		return helpFunc("Replied message is not a media/document.")
	}
	inputLoc, fInfo, err := utils.GetInputFileLocationAndSize(repMsg.Media)
	if err != nil {
		return helpFunc(err.Error())
	}
	var folder, fileName, tgFileName string

	if fInfo != nil {
		tgFileName = fInfo.Name
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
			argparser.CreateDStringFlag(
				"dir",
				"d",
				"DIR (Must be in double quotes.)",
			),
			argparser.CreateDStringFlag(
				"file",
				"f",
				"File Name (Must be in double quotes.)",
			),
			argparser.CreateDBoolFlag(
				"home",
				"h",
				"(To download to current working directory.).",
			),
			argparser.CreateDBoolFlag(
				"caption",
				"c",
				"(To use download with caption instead of filename.)",
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
		dir := pargs["dir"].(string)
		file := pargs["file"].(string)
		// home := pargs["home"].(bool)
		caption := pargs["caption"].(bool)
		if dir != "" && !strings.HasSuffix(dir, "/") {
			dir += "/"
		}
		folder = dir
		if caption {
			name := repMsg.Text
			if !strings.Contains(name, "\n") {
				fileName = name
			}
		} else if file != "" {
			fileName = file
		}
	} else {
		if args != "" {
			folder, fileName = filepath.Split(args)
		}
	}
	if tgFileName == "" {
		tgFileName = "image" + time.Now().Format("2006-01-02_15-04-05") + ".jpg"
	}
	if fileName == "" {
		fileName = tgFileName
	}
	sMsg, err := msg.ReplyText("...")
	if err != nil {
		return nil
	}
	id := fmt.Sprintf("%d:%d", msg.ChatID, msg.ID)
	download := downloaders.Downloader{
		Msg:      sMsg,
		InputLoc: inputLoc,
		Folder:   folder,
		FileName: fileName,
		Size:     fInfo.Size,
	}
	download.Init(id, msg.SenderID)
	err = download.Start()
	if err != nil {
		_, _ = msg.ReplyText("Download failed due to Error:\n" + err.Error())
	} else {
		_, _ = msg.ReplyText(fmt.Sprintf(
			"__Saved to__ `%s` __successfully!__",
			download.Path,
		))
	}
	return nil
}
