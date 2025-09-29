// Package encoders handles encoding via shell
package encoders

import (
	"bytes"
	"enc/config"
	"enc/internal/bot/types"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
)

const CancelCallbackData = "skip"
const CallbackData = "pres"
const SCallbackData = "stats"
const defEncMsg = "**Currently Encoding %s:**\n└`%s`\n\n%s**⏳This Might Take A While⏳**"

type Encoder struct {
	Msg     *tele.Message
	LMsg    *tele.Message
	cmd     *exec.Cmd
	process *os.Process
	Job     *utils.EncodeJob
	STime  time.Time
	clean   bool
	Sender  int64
	ID      string
	LogID   string
}

func(e *Encoder) Start(cmdline string) error {
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
	return nil
}

func (e *Encoder) Callback(dlPath, enPath, text string) {
	e.clean = true
	tInfo := &types.TransferInfo{
		Path: dlPath,
		Sender: e.Sender,
		Time: e.STime,
		ID: e.ID,
	}
	cInfo := &types.CallbackInfo{
		TransferInfo: tInfo,
		Process: e.process,
		OutPath: enPath,
	}
	_ = utils.StoreCallbackInfo(cInfo, tInfo.ID)
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
				[]byte(SCallbackData + "1"),
				),
			),
		markup.Row(
			markup.Callback(
				"Cancel",
				[]byte(CancelCallbackData +"0"),
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
		tInfo := &types.TransferInfo{
			Path: dlPath,
			Sender: e.Sender,
			Time: e.STime,
			ID: e.LogID,
		}
		cInfo := &types.CallbackInfo{
			TransferInfo: tInfo,
			Process: e.process,
			OutPath: enPath,
		}
		_ = utils.StoreCallbackInfo(cInfo, tInfo.ID)
		sender := "Unknown"
		if user, err := tele.GetUser(e.Sender); err != nil {
			sender  = user.FirstName
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
					[]byte(CancelCallbackData +"0"),
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

func (e *Encoder) AwaitCompletion() (stdout, stderr string, err error) {
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
	stderr = errBuf.String()
	stdout = outBuf.String()
	close(ch)
	return
}
