package transfer

import (
	"context"
	"fmt"
	//"os"
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
	"enc/workers/encoders"
	"enc/workers/uploaders"
)

func Mux(ctx *tele.Context, update *tele.Update, msg *tele.Message, args string) error {
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
	if args == "" || utils.SendHelp(args) {
		args = "\n--help"
	}
	var ffmpegArgs string
	if ffargs, flags, found := strings.Cut(args, "\n"); found {
		ffmpegArgs = ffargs
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
				"force",
				"F",
				"To force output file-name.",
			),
			argparser.CreateDInt64Flag(
				"chat",
				"D",
				"id of chat to dump the resulting file.",
			),
			argparser.CreateDBoolFlag(
				"clean",
				"C",
				"to delete command after muxing.",
			),
			argparser.CreateDStringFlag(
				"quality",
				"k",
				"Custom codec & quality",
			),
			argparser.CreateDStringFlag(
				"emoji",
				"e",
				"customize emoji in caption",
			),
			argparser.CreateDStringFlag(
				"ext",
				"E",
				"force change extension",
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
			argparser.CreateDStringFlag(
				"default-audio",
				"A",
				"iso3 of the audio language to default. if there are multiple matching languages the first is selected.",
			),
			argparser.CreateDStringFlag(
				"default-subtitle",
				"S",
				"iso3 of the subtitle language to default. if there are multiple matching languages the first is selected.",
			),
			argparser.CreateDStringFlag(
				"select",
				"s",
				"File id (Specific file to mux) (Only works for links)",
			),
			argparser.CreateDStringFlag(
				"select-input",
				"I",
				"File id (Specific file to mux) (Only works for input link)",
			),
			argparser.CreateDStringFlag(
				"version",
				"v",
				"Tag file with version.",
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
			argparser.CreateDBoolFlag(
				"qb-input",
				"Q",
				"(To download input using qbittorrent instead of aria2.)",
			),
			argparser.CreateDBoolFlag(
				"ytdl-input",
				"Y",
				"(To download input using yt-dlp instead of aria2.)",
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
	} else {
		ffmpegArgs = args
	}
	ffmpegArgs = strings.TrimSuffix(ffmpegArgs, " ")
	var link string
	message := update.EffectiveMessage
	if msg.ReplyToID != 0 {
		err = message.SetRepliedToMessage(ctx, ctx.Raw, ctx.PeerStorage)
		if err != nil {
			_, _ = msg.ReplyText(err.Error())
			return nil
		}
	} else {
		return helpFunc("Reply the media message to mux.")
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
	folder := config.Conf.MuxDir
	var fileName, forcedFileName, iFileName, iSelected, selected, tgFileName, version string
	var emoji, ext, quality, tcaption, tfile string
	var defaultAudio, defaultSubtitle, input2 string
	var ChatID, size int64
	var clean, nparse, qbit, ytdl bool
	if fInfo != nil {
		size = fInfo.Size
		tgFileName = fInfo.Name
	}
	var downloadInput *downloaders.Downloader
	var dlInfo *utils.DLInfo
	sMsg, err := msg.ReplyText("...")
	if err != nil {
		return err
	}
	if len(pargs) != 0 {
		input2 = pargs["input"].(string)
		name := pargs["file-name"].(string)
		forcedFileName = pargs["force"].(string)
		quality = pargs["quality"].(string)
		ChatID = pargs["chat"].(int64)
		emoji = pargs["emoji"].(string)
		ext = pargs["ext"].(string)
		tcaption = pargs["tag-caption"].(string)
		tfile = pargs["tag-file"].(string)
		iSelected = pargs["select-input"].(string)
		selected = pargs["select"].(string)
		version = pargs["version"].(string)
		caption := pargs["caption"].(bool)
		clean = pargs["clean"].(bool)
		defaultAudio = pargs["default-audio"].(string)
		defaultSubtitle = pargs["default-subtitle"].(string)
		nparse = pargs["no-parse"].(bool)
		qbit = pargs["qb"].(bool)
		ytdl = pargs["ytdl"].(bool)
		iQbit := pargs["qb-input"].(bool)
		iYtdl := pargs["ytdl-input"].(bool)
		if name != "" {
			fileName = name
		} else if caption {
			name := repMsg.Text
			if name != "" && !strings.Contains(name, "\n") {
				fileName = name
			}
		}
		if clean {
			defer func() { _ = msg.Delete() }()
		}
		if input2 != "" && (utils.IsURL(input2) || utils.IsMagnet(input2)) {
			var inputLoc tg.InputFileLocationClass
			var size int64
			if strings.HasPrefix(input2, "https://t.me") {
				msg := tele.GetMessageFromLink(input2)
				if msg == nil {
					_, _ = sMsg.EditText("`Could not fetch message from telegram link.`")
					return nil
				}
				if msg.Raw.Media != nil {
					switch msg.Raw.Media.(type) {
					case *tg.MessageMediaWebPage:
						msg.Raw.Media = nil
					default:
					}
				}
				if msg.Raw.Media == nil {
					_, _ = sMsg.EditText("-i: input is not a video/document.")
					return nil
				}
				iLoc, fInfo, err := utils.GetInputFileLocationAndSize(msg.Raw.Media)
				if err != nil {
					_, _ = sMsg.EditText("-i: " + err.Error())
					return nil
				}
				inputLoc = iLoc
				if fInfo != nil {
					size = fInfo.Size
					iFileName = fInfo.Name
				} else {
					iFileName = "image" + time.Now().Format("2006-01-02_15-04-05") + ".jpg"
				}
			} else {
				s := -1
				if iSelected != "" {
					i, err := strconv.Atoi(iSelected)
					if err != nil {
						_, _ = sMsg.EditText("Parameter '-I' requires digits <int> as argument.")
						return nil
					}
					s = i
				}
				if iQbit {
					dlInfo = downloaders.GetQbitDlInfo(input2)
				} else if iYtdl {
					dlInfo = downloaders.GetYdlInfo(input2)
					s -= 1
				} else {
					if utils.IsMegaLink(input2) {
						dlInfo = downloaders.GetMegaDlInfo(input2)
					} else {
						dlInfo = downloaders.GetAria2DlInfo(input2)
						s -= 1
					}
				}
				if dlInfo.Error != nil {
					_, _ = sMsg.EditText("-i: **Error:** " + dlInfo.Error.Error())
					return nil
				}
				if s >= 0 {
					if s > dlInfo.Count-1 {
						_, _ = sMsg.EditText("-I: **Invalid fileID**\n`Total files in folder` :- `" + strconv.Itoa(dlInfo.Count) + "`")
						return nil
					}
					if !iYtdl && !utils.IsSupportedFile(dlInfo.Files[s]) {
						_, _ = sMsg.EditText("-I: " + "File is not in the list of supported files")
						return nil
					}
					iFileName = filepath.Base(dlInfo.Files[s])
				} else if dlInfo.Count == 1 {
					if !iYtdl && !utils.IsSupportedFile(dlInfo.Name) {
						_, _ = sMsg.EditText("-i: File is either not a supported file or it is a folder with a single file. List the link and if it is the latter, select the file with -I.")
						return nil
					}
					iFileName = dlInfo.Name
				} else {
					_, _ = sMsg.EditText("-i: **Link has multiple files, kindly select one with -I**\n`Total files in folder` :- `" + strconv.Itoa(dlInfo.Count) + "`")
					return nil
				}
			}
			downloadInput = &downloaders.Downloader{
				Msg:      sMsg,
				InputLoc: inputLoc,
				Folder:   folder,
				FileName: iFileName,
				Size:     size,
				Selected: iSelected,
				URI:      input2,
				Qbit:     iQbit,
				Ytdl:     iYtdl,
			}
		}
	}
	if link != "" {
		s := -1
		if selected != "" {
			i, err := strconv.Atoi(selected)
			if err != nil {
				_, _ = sMsg.EditText("Parameter '-s' requires digits <int> as argument.")
				return nil
			}
			s = i
		}
		if qbit {
			dlInfo = downloaders.GetQbitDlInfo(link)
		} else if ytdl {
			dlInfo = downloaders.GetYdlInfo(link)
			s -= 1
		} else if utils.IsMegaLink(link) {
			dlInfo = downloaders.GetMegaDlInfo(link)
		} else {
			dlInfo = downloaders.GetAria2DlInfo(link)
			s -= 1
		}
		if dlInfo.Error != nil {
			_, _ = sMsg.EditText("**Error:** " + dlInfo.Error.Error())
			return nil
		}
		if s >= 0 {
			if s > dlInfo.Count-1 {
				_, _ = sMsg.EditText("-s: **Invalid fileID**\n`Total files in folder` :- `" + strconv.Itoa(dlInfo.Count) + "`")
				return nil
			}
			if !ytdl && !utils.IsSupportedFile(dlInfo.Files[s]) {
				_, _ = sMsg.EditText("-s: " + "File is not in the list of supported files")
				return nil
			}
			fileName = filepath.Base(dlInfo.Files[s])
			// iFileName = dlInfo.Files[s]
		} else if dlInfo.Count == 1 {
			if !ytdl && !utils.IsSupportedFile(dlInfo.Name) {
				_, _ = sMsg.EditText("File is either not a supported file or it is a folder with a single file. List the link and if it is the latter, select the file with -s.")
				return nil
			}
			fileName = dlInfo.Name
		} else {
			_, _ = sMsg.EditText("**Link has multiple files, kindly select one with -s**\n`Total files in folder` :- `" + strconv.Itoa(dlInfo.Count) + "`")
			return nil
		}
	}
	download := &downloaders.Downloader{
		Msg:      sMsg,
		InputLoc: inputLoc,
		Folder:   folder,
		FileName: fileName,
		Selected: selected,
		Size:     size,
		URI:      link,
		Qbit:     qbit,
		Ytdl:     ytdl,
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
	waitQueue := bot.WaitQueueM
	waitQueue.Enqueue(id)
	if waitQueue.Waiting() {
		if res, _ := waitQueue.WaitForTurn(context.Background(), id, sMsg); res == 0 {
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
	if downloadInput != nil {
		downloadInput.Init(id, msg.SenderID)
		err = downloadInput.Start()
		if err != nil || downloadInput.IsCancelled {
			_, _ = downloadInput.ReportStatus()
			return nil
		}
		defer downloadInput.CleanDownload()
	}
	if ext == "" {
		ext = filepath.Ext(download.Path)
	}
	baseName := filepath.Base(download.Path)
	tFile := filepath.Join(folder, utils.RemoveExt(baseName))
	tFile2 := tFile + " [Metadata]" + ext
	tFile += " [Temp]" + ext

	if downloadInput != nil {
		ffmpegArgs = "-i " + utils.SafePath(downloadInput.Path) + " " + ffmpegArgs
	}
	ffCmd := fmt.Sprintf(
		"ffmpeg -i %s %s %s -y",
		utils.SafePath(download.Path),
		ffmpegArgs, utils.SafePath(tFile),
	)
	encode := &encoders.Encoder{
		Msg:    sMsg,
		STime:  time.Now(),
		Sender: msg.SenderID,
		ID:     id,
	}
	if err := encode.Start(ffCmd); err != nil {
		_, _ = sMsg.Edit("Error while muxing: "+err.Error(), nil)
		return nil
	}

	encode.ExtraFlags = "mux"
	encode.Callback(download.Path, tFile, "%s **Currently Muxing:**\n└`%s`\n\n`using provided parameters…`%s")
	defer encode.CleanUp()
	_ = encode.AwaitCompletion()
	_ = encode.ReportStatus("Muxing")
	if !encode.Process.Success() {
		utils.SRemove(false, tFile)
		return nil
	}
	download.CleanDownload()
	if downloadInput != nil {
		downloadInput.CleanDownload()
	}
	defer utils.SRemove(false, tFile)
	if !utils.IsVideoFile(tFile) || forcedFileName != "" {
		nparse = true // disable anilist on non video file or forcedFileName
	}
	pOptions := &parser.ParseOptions{
		Direct:      forcedFileName,
		UseAniList:  !nparse,
		Version:     version,
		Path:        tFile,
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
	var thumbPath string
	if !nparse {
		thumb := &utils.TempFile{}
		defer func() {
			if err := thumb.Remove(); err != nil {
				logger.Warn(err)
			}
		}()
		if err := thumb.Create(".jpg"); err != nil {
			logger.Warn(err)
		} else {
			thumbPath = thumb.Name
		}
	}
	r := parser.DynamicThumb(context.Background(), fileName, thumbPath, !nparse, nil)
	if !r.Downloaded {
		thumbPath = ""
	}
	var args2 string
	for arg := range strings.SplitSeq(ffmpegArgs, "-") {
		if strings.Contains(arg, "metadata") {
			args2 += "-" + arg + " "
		}
	}
	if strings.Contains(args2, "This Episode") && r.Title != "" {
		strR := r.Title
		if r.Episode != "" {
			strR = fmt.Sprintf("Episode %s of %s", r.Episode, r.Title)
		}
		if r.Season != "" {
			strR += " Season " + r.Season
		}
		args2 = strings.ReplaceAll(args2, "This Episode", strR)
	}
	if strings.Contains(args2, "Fileinfo") {
		args2 = strings.ReplaceAll(args2, "Fileinfo", result.MediaInfoName)
	}
	if defaultAudio != "" || defaultSubtitle != "" {
		aPosInStm, sPosInStm := utils.PosInStream(tFile, defaultAudio, "", "")
		if defaultAudio != "" {
			args2 += " -disposition:a 0"
			if aPosInStm >= 0 {
				args2 += " -disposition:a:" + strconv.Itoa(aPosInStm) + " default"
			}
		}
		if defaultSubtitle != "" {
			args2 += " -disposition:s 0"
			if aPosInStm >= 0 {
				args2 += " -disposition:s:" + strconv.Itoa(sPosInStm) + " default"
			}
		}

	}
	ffCmd = fmt.Sprintf("ffmpeg -i %s -map 0:v? -map 0:a? -map 0:s? -map 0:t? %s -codec copy %s -y",
		utils.SafePath(tFile), args2, utils.SafePath(tFile2))
	encode = &encoders.Encoder{
		Msg:    sMsg,
		STime:  time.Now(),
		Sender: msg.SenderID,
		ID:     id,
	}
	if err := encode.Start(ffCmd); err != nil {
		_, _ = sMsg.Edit("Error while editing metadata: "+err.Error(), nil)
		return nil
	}
	_ = encode.AwaitCompletion()
	_ = encode.ReportStatus("Editing metadata")
	if !encode.Process.Success() {
		utils.SRemove(false, tFile2)
		return nil
	}
	defer utils.SRemove(false, tFile2)
	cOptions := &parser.CustCapOptions{
		Path:          tFile2,
		UseAniList:    !nparse,
		CustType:      tcaption,
		Ccd:           emoji,
		Ver:           version,
		CodecOverride: quality,
		Direct:        forcedFileName,
	}
	caption, err := parser.CustCap(fileName, cOptions)
	if err != nil {
		caption = fileName
	}
	upload := &uploaders.Uploader{
		Msg:           sMsg,
		Caption:       caption,
		ThumbPath:     thumbPath,
		FileName:      result.FileName,
		ForceDocument: true,
	}
	upload.Init(id, tFile2, msg.SenderID)
	err = upload.Start()
	if err != nil || upload.IsCancelled {
		_, _ = upload.ReportStatus()
		return nil
	}
	if ChatID != 0 {
		_, _ = sMsg.Forward(ChatID)
	}
	return nil
}
