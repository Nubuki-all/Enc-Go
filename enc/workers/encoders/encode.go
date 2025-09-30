// Package encoders handles encoding via shell
package encoders

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	Msg     *tele.Message
	LMsg    *tele.Message
	cmd     *exec.Cmd
	Process *os.ProcessState
	process *os.Process
	Job     *utils.EncodeJob
	STime   time.Time
	clean   bool
	done    bool
	Sender  int64
	ID      string
	LogID   string
	exe     string
	outPath string
	stderr  string
	stdout  string
}

func (e *Encoder) Start(cmdline string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", cmdline)
	} else {
		cmd = exec.Command("sh", "-c", cmdline)
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	e.cmd = cmd
	e.process = cmd.Process
	e.TransferInfo = &types.TransferInfo{}
	e.exe = strings.Split(cmdline, " ")[0]
	return nil
}

func (e *Encoder) Callback(dlPath, enPath, text string) {
	e.clean = true
	e.outPath = enPath
	e.TransferInfo = &types.TransferInfo{
		Path:   dlPath,
		Sender: e.Sender,
		Time:   e.STime,
		ID:     e.ID,
	}
	cInfo := &types.CallbackInfo{
		TransferInfo: e.TransferInfo,
		Process:      e.process,
		OutPath:      enPath,
	}
	_ = utils.StoreCallbackInfo(cInfo, e.ID)
	var addMsg string
	buttonRows := []tg.KeyboardButtonRow{
		markup.Row(
			markup.Callback(
				"ℹ️",
				[]byte(CallbackData+"0"),
			),
		),
		markup.Row(
			markup.Callback(
				"Progress",
				[]byte(SCallbackData+"0"),
			),
			markup.Callback(
				"Server-info",
				[]byte(SCallbackData+"1"),
			),
		),
		markup.Row(
			markup.Callback(
				"Cancel",
				[]byte(CancelCallbackData+"0"),
			),
		),
	}
	if e.Job != nil && e.Job.PendingInfo().Count > 1 {
		addMsg = fmt.Sprintf("**%s Job**\n└`%s`\n\n", e.Job.Pending(), utils.GetCodec(e.Job.Pending()))
		buttonRows = append(buttonRows,
			markup.Row(
				markup.Callback(
					"❌ all jobs",
					[]byte("j"+CancelCallbackData+"0"),
				),
			),
		)
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
		if e.Job != nil && e.Job.PendingInfo().Count > 1 {
			buttonRows = append(buttonRows,
				markup.Row(
					markup.Callback(
						"CANCEL ALL JOBS",
						[]byte("j"+CancelCallbackData+"0"),
					),
				),
			)
		}
		_, _ = e.LMsg.Edit(
			fmt.Sprintf("**User:**\n└[%s](tg://user?id={%d})\n\n%s**Currently Encoding:**\n└`%s`\n\n**Source File:**\n└`%s`", sender, e.Sender, addMsg, out, filepath.Base(dlPath)),
			&tele.EditOptions{ReplyMarkup: markup.InlineKeyboard(buttonRows...)},
		)
	}
}

func (e *Encoder) AwaitCompletion() (err error) {
	ch := make(chan struct{})
	var outBuf, errBuf bytes.Buffer
	e.cmd.Stderr = &errBuf
	e.cmd.Stdout = &outBuf
	if config.Conf.AllowAction {
		actionInterval := 5 * time.Second
		go func() {
			aTicker := time.NewTicker(actionInterval)
			defer aTicker.Stop()

			for {
				select {
				case <-ch:
					return
				case <-aTicker.C:
					_ = e.Msg.Action("game", 0)
				}
			}
		}()
	}
	err = e.cmd.Wait()
	e.Process = e.cmd.ProcessState
	e.done = true
	e.stderr = errBuf.String()
	e.stdout = outBuf.String()
	close(ch)
	return err
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
			text += "of " + filepath.Base(e.outPath) + " "
		}
		if e.IsCancelled {
			text += "was cancelled"
			if e.Canceller != e.Sender {
				name := "Unknown"
				if user, err := tele.GetUser(e.Canceller); err != nil {
					name = user.FirstName
				}
				text += fmt.Sprintf(" by [%s](tg://user?id=%d).", name, e.Canceller)
			} else {
				text += "."
			}
			_, err := e.Msg.Edit(text, nil)
			return err
		} else {
			text += "Failed. \nLogs available below."
			_, _ = e.Msg.Edit(text, nil)
			if e.LMsg != nil {
				_, _ = e.LMsg.Edit(text, nil)
			}
			var reportee int64
			if bot.ReportFailedEnc {
				reportee = config.Rconf.Owners[0]
				if reportee != e.Msg.ChatID {
					go func(text string, id int64) {
						err := tele.SendMessage(id, text)
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
		text = "**" + text + "**"
		_, _ = e.Msg.Edit(text, nil)
		if e.LMsg != nil {
			_, _ = e.LMsg.Edit(text, nil)
		}
		return nil
	}
}
