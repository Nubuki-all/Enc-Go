package dev

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"enc/utils"
	"enc/utils/tele"
)

const maxLen = 4000

func EvalGoSnippet(code string, msg *tele.Message) (string, error) {
	r, w, err := os.Pipe()
	if err != nil { return "", err }
	i := interp.New(interp.Options{Stdin: r, Stdout: w, Stderr: w, Unrestricted: true})
	if err := i.Use(stdlib.Symbols); err != nil { return "", err }
	if err := i.Use(map[string]map[string]reflect.Value{"msg/msg": {"Msg": reflect.ValueOf(msg)}}); err != nil { return "", err }
	_, evalErr := i.Eval(code)
	_ = w.Close()
	var outBuf bytes.Buffer; _, copyErr := io.Copy(&outBuf, r); output := outBuf.String()
	if evalErr != nil { return output, evalErr }; if copyErr != nil { return output, copyErr }; return output, nil
}

func Eval(msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) && !utils.UserIsDev(msg.SenderID) { return nil }
	if args == "" || utils.SendHelp(args) { _, _ = msg.ReplyText(utils.CodeBlock("Eval command help", false)); return nil }
	bMsg, _ := msg.ReplyText("`Processing…`")
	out, err := EvalGoSnippet(args, msg)
	eval := "Success"; if err != nil { eval = err.Error() } else if out != "" { eval = out }
	if len(eval) > maxLen {
		out = fmt.Sprintf("EVAL:\n%s\n\nOUTPUT:\n%s", args, eval)
		_, _ = msg.UploadDoc(&tele.UploadOptions{Bytes: []byte(out), Name: "eval.txt", Caption: "`eval.txt`", Quote: true})
		_ = bMsg.Delete(); return nil
	}
	text := fmt.Sprintf("<pre language=\"go\">%s</pre>\n\n<pre language=\"Output:\">%s</pre>", args, eval)
	_, _ = bMsg.Edit(text, &tele.EditOptions{HTMLParseMode: true}); return nil
}
