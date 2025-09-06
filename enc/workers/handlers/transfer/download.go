package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/logger"
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
	var pargs map[string]interface{}
	helpMsg := "Downloads the replied media/link: to a location (specified or the ALT_DOWNLOAD_DIR env var) locally\n" +
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
				"File Name (Must be in double quotes.) (Doesn't work for links)",
			),
			argparser.CreateDStringFlag(
				"select",
				"s",
				"File id (Specific file to download) (Only works for links)",
			),
			argparser.CreateDBoolFlag(
				"cwd",
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
		pargs = arg.Values
	}
	var link string
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
	if repMsg.Media != nil {
		switch repMsg.Media.(type) {
		case *tg.MessageMediaWebPage:
			repMsg.Media = nil
		}
	}
	if repMsg.Media == nil {
		if link = repMsg.Text; link == "" {
			return helpFunc("Replied message is not a media/document/link.")
		}
		if !utils.IsURL(link) && !utils.IsMagnet(link) {
			return helpFunc("`Not a valid link`.")
		}
	}
	inputLoc, fInfo, err := utils.GetInputFileLocationAndSize(repMsg.Media)
	if err != nil {
		return helpFunc(err.Error())
	}
	var folder, fileName, selected, tgFileName string
	var size int64

	if fInfo != nil {
		size = fInfo.Size
		tgFileName = fInfo.Name
	}
	if strings.HasPrefix(args, "-") {
		dir := pargs["dir"].(string)
		file := pargs["file"].(string)
		home := pargs["cwd"].(bool)
		caption := pargs["caption"].(bool)
		selected = pargs["select"].(string)
		if dir != "" {
			folder = dir
		} else if home {
			path, err := os.Getwd()
			if err == nil {
				folder = path
			} else {
				logger.Error(err)
			}
		}
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
	if link == "" {
		if tgFileName == "" {
			tgFileName = "image" + time.Now().Format("2006-01-02_15-04-05") + ".jpg"
		}
		if fileName == "" {
			fileName = tgFileName
		}
	} else {
		fileName = link
	}
	if folder == "" {
		folder = config.Conf.AltDownloadDir
	}
	sMsg, err := msg.ReplyText("...")
	if err != nil {
		return nil
	}
	id := fmt.Sprintf("%d:%d", sMsg.ChatID, sMsg.ID)
	download := &downloaders.Downloader{
		Msg:      sMsg,
		InputLoc: inputLoc,
		Folder:   folder,
		FileName: fileName,
		Size:     size,
		Selected: selected,
		Uri:      link,
	}
	download.Init(id, msg.SenderID)
	err = download.Start()
	if err != nil || download.IsCancelled {
		_, _ = download.ReportStatus()
	} else {
		_, _ = sMsg.Edit(
			fmt.Sprintf(
				"__Saved to__ `%s` __successfully!__",
				download.Path,
			),
			&tele.EditOptions{},
		)
	}
	return nil
}
