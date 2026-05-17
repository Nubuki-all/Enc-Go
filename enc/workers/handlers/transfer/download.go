// Package transfer contains that strictly uploads or downloads
package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

func Download(msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	cmd := "cmd"
	var err error
	var pargs map[string]any
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
			_, _ = msg.Reply(
				utils.CodeBlock(out, true),
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
	if msg.ReplyToID != 0 {
		err = msg.SetRepliedToMessage()
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
	} else {
		return helpFunc("Reply the media message to download.")
	}
	if repMsg.Raw.Media != nil {
		switch repMsg.Raw.Media.(type) {
		case *tg.MessageMediaWebPage:
			repMsg.Raw.Media = nil
		default:
		}
	}
	if repMsg.Raw.Media == nil {
		if link = repMsg.Text; link == "" {
			return helpFunc("Replied message is not a media/document/link.")
		}
		if !utils.IsURL(link) && !utils.IsMagnet(link) {
			return helpFunc("`Not a valid link`.")
		}
	}
	inputLoc, fInfo, err := utils.GetInputFileLocationAndSize(repMsg.Raw.Media)
	if err != nil {
		return helpFunc(err.Error())
	}
	var folder, fileName, selected, tgFileName string
	var qbit, ytdl bool
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
		qbit = pargs["qb"].(bool)
		ytdl = pargs["ytdl"].(bool)
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
		URI:      link,
		Qbit:     qbit,
		Ytdl:     ytdl,
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

func Rename(msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	cmd := "cmd"
	var err error
	var pargs map[string]any
	helpMsg := "Reply to a file/link to download,rename and upload it.\n" +
		"Either pass the file name as argument or use the below flags;\n" +
		"Available flags:"
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
				"file-name",
				"n",
				"FileName to parse/rename as.",
			),
			argparser.CreateDStringFlag(
				"quality",
				"l",
				"quality in codec.",
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
				"d",
				"disables anilist parsing.",
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
			_, _ = msg.Reply(
				utils.CodeBlock(out, true),
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
	if msg.ReplyToID != 0 {
		err = msg.SetRepliedToMessage()
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
	} else {
		return helpFunc("Reply the media message to rename.")
	}
	if repMsg.Raw.Media != nil {
		switch repMsg.Raw.Media.(type) {
		case *tg.MessageMediaWebPage:
			repMsg.Raw.Media = nil
		default:
		}
	}
	if repMsg.Raw.Media == nil {
		if link = repMsg.Text; link == "" {
			return helpFunc("Replied message is not a media/document/link.")
		}
		if !utils.IsURL(link) && !utils.IsMagnet(link) {
			return helpFunc("`Not a valid link`.")
		}
	}
	inputLoc, fInfo, err := utils.GetInputFileLocationAndSize(repMsg.Raw.Media)
	if err != nil {
		return helpFunc(err.Error())
	}
	folder := config.Conf.TempDir
	var direct, fileName, selected, tgFileName, version string
	var emoji, quality, tcaption, tfile string
	var size int64
	var force, nparse, zIdx bool
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
		if selected != "" && !utils.IsNumber(selected) {
			_, _ = sMsg.EditText("-s: argument accepts Only digits not: " + selected)
			return nil
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
				zIdx = true
			} else if ytdl {
				dlInfo = downloaders.GetYdlInfo(link)
			} else {
				if utils.IsMegaLink(link) {
					dlInfo = downloaders.GetMegaDlInfo(link)
					zIdx = true
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
				zIdx = true
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
		if selected != "" {
			s, _ := strconv.Atoi(selected)
			if !zIdx {
				s -= 1
			}
			if s > dlInfo.Count-1 {
				_, _ = sMsg.EditText("-s: **Invalid file ID**")
				return nil
			}
			fileName = dlInfo.Files[s]
		} else {
			fileName = dlInfo.Name
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
		_, _ = sMsg.Edit(
			"**Parsing failed. check logs for more details**",
			nil,
		)
		logger.Error(err)
		return nil
	}
	if result.FileName != download.FileName {
		_, _ = sMsg.Edit(
			"Renaming:\n`"+download.FileName+"`\n >>>\n`"+result.FileName+"`…",
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

func MediaInfo(msg *tele.Message, args string) error {
	if !utils.UserIsAllowed(msg.SenderID) {
		return nil
	}
	cmd := "cmd"
	var err error
	var pargs map[string]any
	helpMsg := "Downloads the replied message and gets its media info.\n" +
		"Available arguments:\n"
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
				"select",
				"s",
				"File id (Specific file to download) (Only works for links)",
			),
			argparser.CreateDBoolFlag(
				"txt",
				"t",
				"Sends mediainfo as txt file",
			),
			argparser.CreateDBoolFlag(
				"full",
				"f",
				"To get a more verbose media info",
			),
			argparser.CreateDBoolFlag(
				"qb",
				"q",
				"To download a link using qbittorrent instead of aria2.",
			),
			argparser.CreateDBoolFlag(
				"ytdl",
				"y",
				"To download a link using yt-dlp instead of aria2.",
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
		pargs = arg.Values
	}
	var link string
	if msg.ReplyToID != 0 {
		err = msg.SetRepliedToMessage()
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
	} else {
		return helpFunc("Reply the media message to download.")
	}
	if repMsg.Raw.Media != nil {
		switch repMsg.Raw.Media.(type) {
		case *tg.MessageMediaWebPage:
			repMsg.Raw.Media = nil
		default:
		}
	}
	if repMsg.Raw.Media == nil {
		if link = repMsg.Text; link == "" {
			return helpFunc("Replied message is not a media/document/link.")
		}
		if !utils.IsURL(link) && !utils.IsMagnet(link) {
			return helpFunc("`Not a valid link`.")
		}
	}
	inputLoc, fInfo, err := utils.GetInputFileLocationAndSize(repMsg.Raw.Media)
	if err != nil {
		return helpFunc(err.Error())
	}
	folder := config.Conf.MediaInfoDir
	var fileName, selected string
	var asFile, full, qbit, ytdl bool
	var size int64

	if fInfo != nil {
		size = fInfo.Size
		fileName = fInfo.Name
	}
	if len(pargs) != 0 {
		selected = pargs["select"].(string)
		asFile = pargs["txt"].(bool)
		full = pargs["full"].(bool)
		qbit = pargs["qb"].(bool)
		ytdl = pargs["ytdl"].(bool)
		if selected != "" && !utils.IsNumber(selected) {
			_, _ = msg.ReplyText("Parameter '-s' requires digits <int> as argument.")
			return nil
		}
	}
	if link == "" {
		if fileName == "" {
			fileName = "image" + time.Now().Format("2006-01-02_15-04-05") + ".jpg"
		}
	} else {
		fileName = link
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
		URI:      link,
		Qbit:     qbit,
		Ytdl:     ytdl,
	}
	download.Init(id, msg.SenderID)
	err = download.Start()
	if err != nil || download.IsCancelled {
		_, _ = download.ReportStatus()
		return nil
	}
	defer download.CleanDownload()
	_, _ = sMsg.Edit("__Generating media info for__ `"+download.Path+"` __...__", nil)
	mInfo, err := utils.MediaInfoText(ctx, download.Path, full)
	if err != nil {
		_, _ = sMsg.Edit("Error: "+err.Error(), nil)
		return nil
	}
	if asFile {
		_, _ = sMsg.UploadDoc(&tele.UploadOptions{
			Bytes:   []byte(mInfo),
			Name:    "mediainfo.txt",
			Caption: "MediaInfo for " + download.FileName,
			Edit:    true,
		})
		return nil
	}
	page, err := utils.PostToTelegraph("MediaInfo", utils.ParseMediaInfoTextToHTML(mInfo))
	if err != nil {
		_, _ = sMsg.Edit("Error: "+err.Error(), nil)
		return nil
	}
	_, _ = sMsg.EditText(fmt.Sprintf("Mediainfo for: **[%s](%s)**", download.FileName, page.Url))
	return nil
}
