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
	"enc/internal/bot"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/parser"
	"enc/utils/tele"
	"enc/workers/downloaders"
	"enc/workers/uploaders"
)

func Mux(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	var err error
	var pargs map[string]any
	helpMsg := "Remuxes/encodes a replied video/link.\n" +
		"    - Does not recover from database during restart\n" +
		"Required arguments:\n" +
		"    valid ffmpeg parameters without the 'ffmpeg', '-i' or output\n" +
		"Optional arguments:\n" +
		"   Optional arguments are passed seperated from the required argument with a newline"
	helpFunc := func(s string) error {
		_, err := msg.ReplyText(
			s + "\nNeed help? Send `" +
				cmd + " --help`",
		)
		return err
	}
	if args == "" {
		args = "--help"
	}
	if _, flags, found := strings.Cut(args, "\n"); found {
		argList, err := utils.SplitQuotedFieldsErr(flags)
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
		arg := argparser.ParseFlags(
			cmd,
			helpMsg,
			argList,
			argparser.CreateDStringFlag(
				"input",
				"i",
				"Link of file to download, tg link also supported(must be a supergroup link to file)",
			),
			argparser.CreateDStringFlag(
				"file-name",
				"f",
				"To change download name",
			),
			argparser.CreateDStringFlag(
				"emoji",
				"e",
				"customize emoji in caption",
			),
			argparser.CreateDStringFlag(
				"tag-caption",
				"c",
				"specify caption type tag.",
			),
			argparser.CreateDStringFlag(
				"tag-file",
				"t",
				"specify file language tag.",
			),
			argparser.CreateDBoolFlag(
				"force",
				"f",
				"force rename a file to a specified filename and file type.",
			),
			argparser.CreateDStringFlag(
				"select",
				"s",
				"File id (Specific file to rename) (Only works for links)",
			),
			argparser.CreateDStringFlag(
				"version",
				"v",
				"specify a number for versioning.",
			),
			argparser.CreateDBoolFlag(
				"caption",
				"a",
				"(To use download with caption instead of filename.)",
			),
			argparser.CreateDBoolFlag(
				"no-parse",
				"N",
				"to turn off anilist parsing",
			),
			argparser.CreateDBoolFlag(
				"qb",
				"q",
				"(To download using qbittorrent instead of aria2.)",
			),
			argparser.CreateDBoolFlag(
				"ytdl",
				"y",
				"(To download using yt-dlp instead of aria2.)",
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
		return helpFunc("Reply the media message to rename.")
	}
	repMsg := message.ReplyToMessage
	if repMsg.Media != nil {
		switch repMsg.Media.(type) {
		case *tg.MessageMediaWebPage:
			repMsg.Media = nil
		default:
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
	folder := config.Conf.TempDir
	var direct, fileName, selected, tgFileName, version string
	var emoji, quality, tcaption, tfile string
	var size int64
	var force, nparse bool
	if fInfo != nil {
		size = fInfo.Size
		tgFileName = fInfo.Name
	}
	var download *downloaders.Downloader
	var dlInfo *utils.DLInfo
	sMsg, err := msg.ReplyText("...")
	if err != nil {
		return err
	}
	if strings.HasPrefix(args, "-") && len(pargs) != 0 {
		fileName = pargs["file-name"].(string)
		quality = pargs["quality"].(string)
		emoji = pargs["emoji"].(string)
		tcaption = pargs["tag-caption"].(string)
		tfile = pargs["tag-file"].(string)
		selected = pargs["select"].(string)
		version = pargs["version"].(string)
		caption := pargs["caption"].(bool)
		force = pargs["force"].(bool)
		nparse = pargs["no-parse"].(bool)
		qbit := pargs["qb"].(bool)
		ytdl := pargs["ytdl"].(bool)
		if caption {
			name := repMsg.Text
			if name != "" && !strings.Contains(name, "\n") {
				fileName = name
			}
		}
		if force {
			if fileName == "" {
				_, _ = sMsg.EditText("**Force rename to what exactly?**")
				return nil
			}
			direct = fileName
		}
		if link != "" {
			if qbit {
				dlInfo = downloaders.GetQbitDlInfo(link)
			} else if ytdl {
				dlInfo = downloaders.GetYdlInfo(link)
			} else {
				if utils.IsMegaLink(link) {
					dlInfo = downloaders.GetMegaDlInfo(link)
				} else {
					dlInfo = downloaders.GetAria2DlInfo(link)
				}
			}
		}
		download = &downloaders.Downloader{
			Msg:      sMsg,
			InputLoc: inputLoc,
			Folder:   folder,
			FileName: fileName,
			Size:     size,
			Selected: selected,
			URI:      link,
			Qbit:     qbit,
			Ytdl:     ytdl,
		}
	} else {
		if args != "" {
			fileName = args
		}
		if link != "" {
			if utils.IsMegaLink(link) {
				dlInfo = downloaders.GetMegaDlInfo(link)
			} else {
				dlInfo = downloaders.GetAria2DlInfo(link)
			}
		}
		download = &downloaders.Downloader{
			Msg:      sMsg,
			InputLoc: inputLoc,
			Folder:   folder,
			FileName: fileName,
			Size:     size,
			URI:      link,
		}
	}
	if dlInfo != nil {
		if dlInfo.Count > 1 && selected == "" {
			_, _ = sMsg.EditText("**Can not rename multiple files at once, use -s to select a file.**")
			return nil
		}
	}
	if fileName == "" {
		if tgFileName == "" {
			_, _ = sMsg.EditText("__No file name found, cannot proceed.__")
			return nil
		}
		fileName = tgFileName
		download.FileName = tgFileName
	}
	id := fmt.Sprintf("%d:%d", sMsg.ChatID, sMsg.ID)
	waitQueue := bot.WaitQueueR
	waitQueue.Enqueue(id)
	if waitQueue.Waiting() {
		if res, _ := waitQueue.WaitForTurn(ctx, id, sMsg); res == 0 {
			return nil
		}
	}
	defer waitQueue.Remove(id)
	download.Init(id, msg.SenderID)
	err = download.Start()
	if err != nil || download.IsCancelled {
		_, _ = download.ReportStatus()
		return nil
	}
	defer download.CleanDownload()
	pOptions := &parser.ParseOptions{
		Direct:      direct,
		UseAniList:  !nparse,
		Version:     version,
		Path:        download.Path,
		ConOverride: tfile,
		Quality:     quality,
	}
	result, err := parser.ParseName(fileName, pOptions)
	if err != nil {
		_, _ = sMsg.Edit("**Parsing failed. check logs for more details**",
			nil,
		)
		logger.Error(err)
		return nil
	}
	if result.FileName != download.FileName {
		_, _ = sMsg.Edit("Renaming:\n`"+download.FileName+"`\n >>>\n`"+result.FileName+"`…",
			nil,
		)
		newPath := filepath.Join(filepath.Dir(download.Path), result.FileName)
		if err = os.Rename(download.Path, newPath); err != nil {
			logger.Warn(err)
		}

		download.Path = newPath
	}
	var thumbPath string
	if !nparse && direct == "" {
		thumb := &utils.TempFile{}
		defer func() {
			if err := thumb.Remove(); err != nil {
				logger.Warn(err)
			}
		}()
		if err := thumb.Create(".jpg"); err != nil {
			logger.Warn(err)
		} else {
			result := parser.DynamicThumb(ctx, fileName, thumb.Name, true, nil)
			if result.Downloaded {
				thumbPath = thumb.Name
			}
		}
	}
	cOptions := &parser.CustCapOptions{
		Path:          download.Path,
		UseAniList:    !nparse,
		CustType:      tcaption,
		Ccd:           emoji,
		Ver:           version,
		CodecOverride: quality,
		Direct:        direct,
	}
	caption, err := parser.CustCap(fileName, cOptions)
	if err != nil {
		caption = fileName
	}
	upload := &uploaders.Uploader{
		Msg:           sMsg,
		Caption:       caption,
		ThumbPath:     thumbPath,
		ForceDocument: true,
	}
	upload.Init(id, download.Path, msg.SenderID)
	err = upload.Start()
	if err != nil || upload.IsCancelled {
		_, _ = upload.ReportStatus()
	}
	return nil
}
