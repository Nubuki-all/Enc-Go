package dev

import (
	"fmt"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"

	"enc/utils"
	"enc/utils/tele"
)

func GetLogs(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		if !utils.UserIsDev(msg.SenderID) {
			return nil
		}
	}
	helpMsg := "Upload bot's logs in txt format.\n" +
		"Or as a message if an argument is specified." +
		"Arguments:\n"
	arg := argparser.ParseFlags(
		cmd,
		helpMsg,
		argList,
		argparser.CreateDIntFlag(
			"from",
			"f",
			"Line Number to begin from, must be more than 0.",
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
	sNum := pargs["from"].(int)
	logFileName := config.Conf.LogFileName
	if sNum < 1 {
		_, _ = msg.UploadDoc(&tele.UploadOptions{
			Path:  logFileName,
			Quote: true,
		})
		return nil
	}
	var builder strings.Builder
	for i := sNum - 1; i >= 1; i-- {
		line := utils.ReadNToLastLine(logFileName, i)
		if line != "" {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	msgText := builder.String()
	if strings.TrimSpace(msgText) == "" {
		msgText = "Nothing Here.\nTry with a higher number"
	}
	chunks := utils.SplitText(msgText, "\n", false)
	preMsg := msg
	var err error
	for _, chunk := range chunks {
		out := "<pre>" + chunk + "</pre>"
		preMsg, err = preMsg.Reply(out,
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		if err != nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}
