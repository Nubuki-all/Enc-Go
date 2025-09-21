package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"

	"enc/config"
	//"enc/internal/bot"
	//"enc/internal/logger"
	"enc/internal/bot/types"
	"enc/utils"
	"enc/utils/argparser"
	"enc/utils/tele"
	"enc/workers/downloaders"
	"enc/workers/uploaders"
)

const maxFileSize = 2126000000

func Upload(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	cmd := update.Args()[0]
	helpMsg := "Uploads a file/files from local directory or direct/torrent link\n" +
		"Just pass any of the following:\n" +
		"    - the file (with -f) e.g -f \"something.txt\"\n" +
		"    - the folder path\n" +
		"    - direct link\n" +
		"    - torrent/magnet link\n" +
		"Or: "
	var err error
	var pargs map[string]any
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
				"file",
				"f",
				"File or folder to upload",
			),
			argparser.CreateDStringFlag(
				"link",
				"l",
				"direct/torrent/magnet link",
			),
			argparser.CreateDStringFlag(
				"ext",
				"e",
				"Single file uploads only. Change file extension.",
			),
			argparser.CreateDStringFlag(
				"select",
				"s",
				"[File ID] (Specific file to rename) (Only works for links)",
			),
			argparser.CreateDBoolFlag(
				"qbit",
				"q",
				"To download using qbittorrent instead of aria2.",
			),
			argparser.CreateDBoolFlag(
				"mkv",
				"m",
				"same as --ext but only changes to 'mkv'. It is the same as passing --ext \".mkv\"",
			),
			argparser.CreateDBoolFlag(
				"clean",
				"c",
				"cleans command and remits no message as to what or how many files were uploaded.",
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

	// prepare defaults
	var ext, file, link, selected string
	var clean, qbit bool
	// var topicID int64
	uCanMsg := "`Folder upload has been force cancelled`"
	if len(pargs) != 0 {
		link = pargs["link"].(string)
		file = pargs["file"].(string)
		ext = pargs["ext"].(string)
		mkv := pargs["mkv"].(bool)
		clean = pargs["clean"].(bool)
		selected = pargs["select"].(string)
		qbit = pargs["qbit"].(bool)
		if link != "" {
			if !utils.IsURL(link) && !utils.IsMagnet(link) {
				return helpFunc("'--link': `Value passed is not a valid link.`")
			}
		}
		if mkv {
			ext = ".mkv"
		} else if ext != "" && ext[0] == '.' {
			ext += "."
		}
	} else {
		if utils.IsURL(args) || utils.IsMagnet(args) {
			link = args
		} else {
			file = args
		}
	}
	// detect forum topic id if present (mirrors python logic)
	// validate -qs numeric
	if selected != "" {
		if _, perr := strconv.Atoi(selected); perr != nil {
			return helpFunc("'--select': `param accepts only digits`")
		}
	}
	if clean {
		_ = msg.Delete()
	}

	// resolve file argument
	if link != "" {
		dlFolder := filepath.Join(config.Conf.AltDownloadDir, fmt.Sprintf("%d:%d", msg.ChatID, msg.ID))
		sMsg, err := msg.ReplyText("…")
		if err != nil {
			return err
		}
		dID := fmt.Sprintf("%d:%d", sMsg.ChatID, sMsg.ID)
		download := &downloaders.Downloader{
			Msg:      sMsg,
			Folder:   dlFolder,
			FileName: link,
			Selected: selected,
			URI:      link,
			Qbit:     qbit,
		}
		download.Init(dID, msg.SenderID)
		if err := download.Start(); err != nil || download.IsCancelled {
			_, _ = download.ReportStatus()
			return nil
		}
		defer download.CleanDownload()
		defer func() { _ = os.RemoveAll(dlFolder) }()
		_ = sMsg.Delete()
		file = download.Path
	}

	chainMsg := msg

	// ---- handle folder uploads ----
	if utils.DirExists(file) {
		buttonRows := []tg.KeyboardButtonRow{
			markup.Row(
				markup.Callback(
					"Cancel",
					[]byte("cancel_all_upload"),
				),
			),
		}
		ctrlMsg, _ := msg.Reply("Folder upload control panel.",
			&tele.ReplyOptions{
				ReplyMarkup: markup.InlineKeyboard(buttonRows...),
			},
		)
		ctrlID := fmt.Sprintf("%d:%d", ctrlMsg.ChatID, ctrlMsg.ID)
		dirUpload := &types.TransferInfo{ID: ctrlID, Sender: msg.SenderID}
		_ = utils.StoreCallbackInfo(&types.CallbackInfo{
			TransferInfo: dirUpload,
		}, ctrlID,
		)
		defer utils.DelCallbackInfo(ctrlID)
		defer ctrlMsg.Delete() //nolint:errcheck
		fJump := msg
		uploadedCount := 0

		// iterate recursively
		err = filepath.Walk(file, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				// skip unreadable paths
				return nil
			}
			// only process directories containing files by scanning current dir
			if !info.IsDir() {
				return nil
			}
			entries, _ := os.ReadDir(path)
			var files []string
			for _, e := range entries {
				if !e.IsDir() {
					files = append(files, e.Name())
				}
			}
			sort.Strings(files)
			if len(files) == 0 {
				// send "empty" note
				_, _ = msg.ReplyText(fmt.Sprintf("`📁 %s is empty.`", path))
				return nil
			}
			total := len(files)
			idx := 1

			if clean && msg.TopicID != 0 {
				_, _ = msg.ReplyText(fmt.Sprintf("**%s**", filepath.Base(path)))
			}

			for _, name := range files {
				// cancellation check: UCancelled returns list-like; adapt accordingly
				if dirUpload.IsCancelled {
					if !clean {
						_, _ = msg.ReplyText(uCanMsg)
					}
					return fmt.Errorf("cancelled")
				}

				fullPath := filepath.Join(path, name)
				if utils.SizeOf(fullPath) > maxFileSize {
					chainMsg, _ = chainMsg.ReplyText(fmt.Sprintf("Uploading of `%s` failed because file was larger than 2GB", name))
					continue
				}

				// upload loop (retry on certain recoverable errors)
				for {
					// cancellation checked in loop condition
					if dirUpload.IsCancelled {
						if !clean {
							_, _ = msg.ReplyText(uCanMsg)
						}
						return fmt.Errorf("cancelled")
					}

					caption := fmt.Sprintf("`%s`", name)
					ulMsg, err := chainMsg.ReplyText(fmt.Sprintf("**Uploading:-**\n`%s`\n**from 📁:**\n `%s`\n`(%d/%d)`…", name, path, idx, total))
					if err != nil {
						return err
					}
					uID := fmt.Sprintf("%d:%d", ulMsg.ChatID, ulMsg.ID)
					time.Sleep(10 * time.Second)

					upload := &uploaders.Uploader{
						Msg:     ulMsg,
						Caption: caption,
					}
					upload.Init(uID, fullPath, msg.SenderID)
					uerr := upload.Start()
					if uerr != nil {
						// To-Do: Retry on errors
						// unrecoverable: break and report
						_, _ = ulMsg.Edit(fmt.Sprintf("Uploading of `%s` failed: %v", name, uerr), nil)
						break
					}
					if !upload.IsCancelled {
						chainMsg = ulMsg
						uploadedCount++
					} else {
						if !clean {
							_, _ = ulMsg.Edit(fmt.Sprintf("Uploading of `%s` was cancelled.", name), nil)
						} else {
							_ = ulMsg.Delete()
						}
					}
					idx++
					break
				} // end upload retry loop
			} // end files loop

			// small pause between directories to mimic original
			time.Sleep(10 * time.Second)
			if !clean {
				if fJumpMsg, _ := fJump.ReplyText(fmt.Sprintf("`All files in`:\n'`%s`'\n`have been uploaded successfully.`", path)); fJumpMsg != nil {
					fJump = fJumpMsg
				}
			}
			time.Sleep(1 * time.Second)
			return nil
		})
		if err != nil {
			// if cancelled we returned earlier with error; swallow cancellation error
			if err.Error() == "cancelled" {
				return nil
			}
			// otherwise continue and return the error
			return err
		}
	} else if !utils.FileExists(file) {
		_, _ = msg.ReplyText("__File or folder not found__")
	} else {
		// ---- single file upload ----
		rMsg, _ := msg.ReplyText(fmt.Sprintf("`Uploading %s…`", filepath.Base(file)))
		f := filepath.Base(file)
		uID := fmt.Sprintf("%d:%d", rMsg.ChatID, rMsg.ID)

		// extension override: copy to folder with new name
		if ext != "" {
			extension := filepath.Ext(f)
			baseName := strings.TrimSuffix(f, extension)
			f = baseName + ext
		}
		caption := "`" + f + "`"
		upload := &uploaders.Uploader{
			Msg:      rMsg,
			FileName: f,
			Caption:  caption,
			// ThumbPath:     thumbPath,
			ForceDocument: true,
		}
		upload.Init(uID, file, msg.SenderID)
		err = upload.Start()
		if err != nil || upload.IsCancelled {
			if clean {
				_ = rMsg.Delete()
			} else {
				_, _ = upload.ReportStatus()
			}
		}
	}

	return nil
}
