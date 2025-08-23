package dev

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/celestix/gotgproto/ext"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"enc/internal/logger"
	"enc/utils"
	"enc/utils/tele"
)

// EvalGoSnippet runs a Go snippet and returns its stdout and the returned error (if any).
// The snippet should be full Go statements (e.g. `import "fmt"; fmt.Println("hello")` or `package main; func main(){...}`).
func EvalGoSnippet(code string, msg *tele.Message) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	i := interp.New(interp.Options{
		Stdin:        r,
		Stdout:       w,
		Stderr:       w,
		Unrestricted: true,
	})
	if err := i.Use(stdlib.Symbols); err != nil {
		return "", err
	}

	if err := i.Use(map[string]map[string]reflect.Value{
		"msg/msg": {
			"Msg": reflect.ValueOf(msg),
		},
	}); err != nil {
		return "", err
	}

	// Evaluate the snippet
	_, evalErr := i.Eval(code)

	// Close the writer
	w.Close()

	// Read the captured output
	var outBuf bytes.Buffer
	_, copyErr := io.Copy(&outBuf, r)

	output := outBuf.String()

	// Prefer returning the evaluation error (so caller sees stack/yaegi error). If no evalErr,
	// but the io.Copy produced an error, return that.
	if evalErr != nil {
		return output, evalErr
	}
	if copyErr != nil {
		return output, copyErr
	}
	return output, nil
}

func Eval(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		if !utils.UserIsDev(msg.SenderID) {
			return nil
		}
	}
	helpMsg := utils.StringFormat(
		"Evaluate and execute code within bot.",
		"Command requires code to execute as arguments.",
		"For example:",
		`println("Hello-World")`,
	)
	if args == "" {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	rawArgs, err := msg.RawText()
	if err == nil {
		_, args, _ = strings.Cut(rawArgs, " ")
		args = strings.Trim(args, " ")
	} else {
		logger.Warn(err)
	}
	out, err := EvalGoSnippet(args, msg)
	var eval string
	if err != nil {
		eval = err.Error()
	} else if out != "" {
		eval = out
	} else {
		eval = "Success"
	}
	text := fmt.Sprintf("```go\n%s\n```\n\n```output\n%s\n```", args, eval)
	_, _ = msg.ReplyText(text)
	return nil
}
