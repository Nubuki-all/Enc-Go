// Package encoders handles encoding via shell
package encoders

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"

	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
)

const (
	CancelCallbackData = "skip"
	CallbackData       = "pres"
	SCallbackData      = "stats"
	defEncMsg          = "**Currently Encoding %s:**\n└`%s`\n\n%s**⏳This Might Take A While⏳**"
)

type Encoder struct {
	*types.TransferInfo
	Context    context.Context
	Msg        *tele.Message
	LMsg       *tele.Message
	cmd        *exec.Cmd
	Process    *os.ProcessState
	process    *types.Process
	CancelJobs bool
	Job        *utils.EncodeJob
	JobPos     string
	JobCodec   string
	ExtraPath  string
	ETime      time.Time
	STime      time.Time
	clean      bool
	done       bool
	Sender     int64
	ID         string
	LogID      string
	exe        string
	outPath    string
	stderr     string
	stdout     string
	errBuf     *bytes.Buffer
	outBuf     *bytes.Buffer
}

func (e *Encoder) Start(cmdline string) error {
	var cmd *exec.Cmd
	if e.Context == nil {
		e.Context = context.Background()
	}
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(e.Context, "cmd", "/C", cmdline)
	} else {
		cmd = exec.CommandContext(e.Context, "sh", "-c", cmdline)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	e.outBuf = &bytes.Buffer{}
	e.errBuf = &bytes.Buffer{}
	cmd.Stdout = e.outBuf
	cmd.Stderr = e.errBuf
	err := cmd.Start()
	if err != nil {
		return err
	}
	e.cmd = cmd
	e.process = types.NewProcess(cmd.Process)
	e.TransferInfo = &types.TransferInfo{}
	e.exe = strings.Split(cmdline, " ")[0]
	return nil
}

func (e *Encoder) Callback(dlPath, enPath, text string) {
	e.clean = true
	e.outPath = enPath
	e.TransferInfo = &types.TransferInfo{
		Path:       dlPath,
		Sender:     e.Sender,
		Time:       e.STime,
		ID:         e.ID,
		ExtraFlags: e.ExtraFlags,
	}
	cInfo := &types.CallbackInfo{
		TransferInfo: e.TransferInfo,
		Process:      e.process,
		OutPath:      enPath,
		DlPath:       e.ExtraPath,
	}
	_ = utils.StoreCallbackInfo(cInfo, e.ID)
	var addMsg string
	buttonRows := []tg.KeyboardButtonRow{
		markup.Row(
			markup.Callback("ℹ️", []byte(CallbackData+"0")),
		),
		markup.Row(
			markup.Callback("Progress", []byte(SCallbackData+"0")),
			markup.Callback("Server-info", []byte(SCallbackData+"1")),
		),
		markup.Row(
			markup.Callback("Cancel", []byte(CancelCallbackData+"0")),
		),
	}
	if e.Job != nil && e.Job.PendingInfo().Count > 1 {
		addMsg = fmt.Sprintf(
			"**%s Job**\n└`%s`\n\n",
			cmp.Or(e.JobPos, e.Job.PendingPos()),
			cmp.Or(e.JobCodec, utils.GetCodec(e.Job.Pending())),
		)
		if e.CancelJobs {
			buttonRows[len(buttonRows)-1] = markup.Row(
				markup.Callback("Cancel", []byte(CancelCallbackData+"0")),
				markup.Callback("❌ all jobs", []byte("j"+CancelCallbackData+"0")),
			)
		}
	}

	if text == "" {
		text = defEncMsg
	}
	out := filepath.Base(enPath)
	_, _ = e.Msg.Edit(
		fmt.Sprintf(text, random.Enmoji(), out, addMsg),
		&tele.EditOptions{ReplyMarkup: markup.InlineKeyboard(buttonRows...)},
	)
	if e.LMsg != nil && e.Sender != 0 {
		if e.LogID == "" {
			e.LogID = fmt.Sprintf("%d:%d", e.LMsg.ChatID, e.LMsg.ID)
		}
		_ = utils.StoreCallbackInfo(cInfo, e.LogID)
		sender := "Unknown"
		if user, err := tele.GetUser(e.Sender); err != nil {
			sender = user.FirstName
		}
		buttonRows := []tg.KeyboardButtonRow{
			markup.Row(
				markup.Callback(
					"ℹ️",
					[]byte(CallbackData+"0"),
				),
			),
			markup.Row(
				markup.Callback(
					"CHECK PROGRESS",
					[]byte(SCallbackData+"2"),
				),
			),
			markup.Row(
				markup.Callback(
					"CANCEL PROCESS",
					[]byte(CancelCallbackData+"0"),
				),
			),
		}
		if e.Job != nil && e.Job.PendingInfo().Count > 1 && e.CancelJobs {
			buttonRows[len(buttonRows)-1] = markup.Row(
				markup.Callback("CANCEL PROCESS", []byte(CancelCallbackData+"0")),
				markup.Callback("CANCEL ALL JOBS", []byte("j"+CancelCallbackData+"0")),
			)
		}
		_, _ = e.LMsg.Edit(
			fmt.Sprintf(
				"<b>User:</b>\n└<a href=\"tg://user?id=%d\"%s</a>\n\n%s<b>Currently Encoding:</b>\n└<code>%s</code>\n\n<b>Source File:</b>\n└<code>%s</code>",
				e.Sender,
				sender,
				addMsg,
				out,
				filepath.Base(dlPath),
			),
			&tele.EditOptions{
				HTMLParseMode: true,
				ReplyMarkup:   markup.InlineKeyboard(buttonRows...),
			},
		)
	}
}

func (e *Encoder) AwaitCompletion() (err error) {
	ch := make(chan struct{})
	if config.Conf.AllowAction {
		actionInterval := 5 * time.Second
		go func() {
			aTicker := time.NewTicker(actionInterval)
			defer aTicker.Stop()

			for {
				select {
				case <-ch:
					_ = e.Msg.Action("", 0)
					return
				case <-aTicker.C:
					_ = e.Msg.Action("game", 0)
				}
			}
		}()
	}
	err = e.cmd.Wait()
	if e.Job != nil && e.Job.Completed {
		e.IsCancelled = true
	}
	e.Process = e.cmd.ProcessState
	e.done = true
	e.ETime = time.Now()
	e.stderr = e.errBuf.String()
	e.stdout = e.outBuf.String()
	close(ch)
	if e.clean {
		e.cleanUp()
	}
	return err
}

func (e *Encoder) cleanUp() {
	cInfo := utils.GetCallbackInfo(e.ID)
	if cInfo == nil {
		return
	}
	m := false
	if e.Job != nil && cInfo.ReturnFlags == "cancel_jobs" {
		e.Job.Complete()
		m = true
	}
	utils.DelCallbackInfo(e.ID)
	if e.LMsg != nil && e.Sender != 0 {
		cInfo = utils.GetCallbackInfo(e.LogID)
		if cInfo == nil {
			return
		}
		if !m && e.Job != nil && cInfo.ReturnFlags == "cancel_jobs" {
			e.Job.Complete()
		}
		utils.DelCallbackInfo(e.LogID)
	}
}

func (e *Encoder) ReportStatus(action string) (err error) {
	if !e.done {
		logger.Warn("ReportStatus was called on an uncompleted encode")
		return nil
	}
	if action == "" {
		action = "Encoding"
	}
	text := action + " "
	if !e.cmd.ProcessState.Success() {
		if e.outPath != "" {
			text += "of <code>" + filepath.Base(e.outPath) + "</code> "
		}
		if e.IsCancelled {
			text += "was cancelled"
			if e.Canceller != e.Sender {
				name := "Unknown"
				if user, err := tele.GetUser(e.Canceller); err != nil {
					name = user.FirstName
				}
				text += fmt.Sprintf(" by <a href=\"tg://user?id=%d.\">%s</a>", e.Canceller, name)
			} else {
				text += "."
			}
			_, err := e.Msg.Edit(text, &tele.EditOptions{HTMLParseMode: true})
			return err
		} else {
			text += "Failed. \nLogs available below."
			_, _ = e.Msg.Edit(text, &tele.EditOptions{HTMLParseMode: true})
			if e.LMsg != nil {
				_, _ = e.LMsg.Edit(text, &tele.EditOptions{HTMLParseMode: true})
			}
			var reportee int64
			if bot.ReportFailedEnc {
				reportee = config.Rconf.Owners[0]
				if reportee != e.Msg.ChatID {
					go func(text string, id int64) {
						_, err := tele.SendMsg(
							id,
							text,
							&tele.SendOptions{HTMLParseMode: true},
						)
						if err != nil {
							logger.Error(err)
						}
					}(text, reportee)
				} else {
					reportee = 0
				}
			}
			var err error
			var fmsg *tele.Message
			out := "Stderr:\n" + e.stderr + "\n\nStdout:\n" + e.stdout
			if len(out) > 4095 {
				fmsg, err = e.Msg.UploadDoc(&tele.UploadOptions{
					Bytes: []byte(out),
					Name:  e.exe + "_error.txt",
					Quote: true,
				})
			} else {
				fmsg, err = e.Msg.ReplyText(out)
			}
			if err != nil {
				return err
			}
			if e.LMsg != nil {
				_, _ = fmsg.Forward(e.LMsg.ChatID)
			}
			if reportee != 0 {
				_, _ = fmsg.Forward(reportee)
			}
			return nil
		}
	} else {
		text += "was successful!"
		text = "<b>" + text + "</b>"
		_, _ = e.Msg.Edit(text, &tele.EditOptions{HTMLParseMode: true})
		if e.LMsg != nil {
			_, _ = e.LMsg.Edit(text, &tele.EditOptions{HTMLParseMode: true})
		}
		return nil
	}
}
