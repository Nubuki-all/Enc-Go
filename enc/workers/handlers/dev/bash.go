package dev

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/celestix/gotgproto/ext"

	"enc/utils"
	"enc/utils/tele"
)

func Bash(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		if !utils.UserIsDev(msg.SenderID) {
			return nil
		}
	}
	helpMsg := "Run bash/system commands in bot\n" +
		"Much care must be taken especially on Local deployment\n" +
		"\n" +
		"USAGE:\n" +
		"Command requires executables as argument\n" +
		`For example "/bash ls"`

	if args == "" || utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg))
		return nil
	}

	// Send initial "Processing..." message (optional)
	bMsg, _ := msg.ReplyText("Processing ...")

	// Prepare command: run under shell so we can pass complex commands
	cmdExec := exec.CommandContext(ctx, "bash", "-lc", args)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmdExec.Stdout = &stdoutBuf
	cmdExec.Stderr = &stderrBuf

	if err := cmdExec.Start(); err != nil {
		out := fmt.Sprintf("Failed to start command: %v", err)
		_, _ = bMsg.EditText(out)
		return err
	}

	pid := -1
	if cmdExec.Process != nil {
		pid = cmdExec.Process.Pid
	}

	_ = cmdExec.Wait()

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if stderr == "" {
		stderr = "No Error"
	}
	if stdout == "" {
		stdout = "Tip:\nIf you want to see the results of your code, print them to stdout."
	}

	OUTPUT := fmt.Sprintf("QUERY:\n__Command:__\n%s\n__PID:__\n%d\n\nstderr:\n%s\n\nOutput:\n%s", args, pid, stderr, stdout)

	// If output is long, upload as a file and send as document
	if len(OUTPUT) > maxLen {
		_, _ = msg.UploadDoc(&tele.UploadOptions{
			Bytes:   []byte(OUTPUT),
			Name:    "exec.txt",
			Caption: args,
			Quote:   true,
		})
		_ = bMsg.Delete()
		return nil
	}
	text := fmt.Sprintf("<pre language=\"bash\">%s</pre>\n\n<i>PID:</i>\n<code>%d</code>\n\n<pre language=\"Stderr:\">%s</pre>\n\n<pre language=\"Output:\">%s</pre>", args, pid, stderr, stdout)
	_, _ = bMsg.Edit(text, &tele.EditOptions{
		HtmlParseMode: true,
	})
	return nil
}
