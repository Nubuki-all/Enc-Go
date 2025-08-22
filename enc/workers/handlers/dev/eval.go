package dev

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/celestix/gotgproto/ext"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"enc/utils"
	"enc/utils/tele"
)

// EvalGoSnippet runs a Go snippet and returns its stdout and the returned error (if any).
// The snippet should be full Go statements (e.g. `import "fmt"; fmt.Println("hello")` or `package main; func main(){...}`).
func EvalGoSnippet(code string) (string, error) {
	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		return "", err
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	var outBuf bytes.Buffer
	done := make(chan error)
	go func() {
		_, err := io.Copy(&outBuf, r)
		done <- err
	}()

	_, err = i.Eval(code)

	// restore
	w.Close()
	os.Stdout = oldStdout
	<-done

	return outBuf.String(), err
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
		`import "fmt"`,
		`fmt.Println("Hello World")`,
	)
	if args == "" {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}
	err, out := EvalGoSnippet(args)
	var eval string
	if err {
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
