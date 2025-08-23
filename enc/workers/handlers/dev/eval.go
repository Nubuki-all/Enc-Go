package dev

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/celestix/gotgproto/ext"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"enc/utils"
	"enc/utils/tele"
)

// EvalGoSnippet runs a Go snippet and returns its stdout and the returned error (if any).
// The snippet should be full Go statements (e.g. `import "fmt"; fmt.Println("hello")` or `package main; func main(){...}`).
func EvalGoSnippet(code string, msg *tele.Message) (string, error) {
	i := interp.New(interp.Options{})
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

	// Capture stdout produced by evaluated code (fmt.Println etc.)
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	os.Stderr = w
	// Ensure stdout is restored even on panic
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	// Evaluate the snippet
	_, evalErr := i.Eval(code)

	// Close the writer and restore stdout
	w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

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
		`import "fmt"`,
		`fmt.Println("Hello World")`,
	)
	if args == "" {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
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
