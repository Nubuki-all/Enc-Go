package dev

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"enc/utils"
	"enc/utils/tele"

	"github.com/celestix/gotgproto/ext"
	"github.com/shirou/gopsutil/v3/process"
)

type ProcCPU struct {
	PID  int32
	User string
	Name string
	CPU  float64
}

func top(limit int) string {
	procs, err := process.Processes()
	if err != nil {
		panic(err)
	}

	for _, p := range procs {
		_, _ = p.CPUPercent()
	}

	time.Sleep(1 * time.Second)

	var list []ProcCPU

	for _, p := range procs {
		cpu, err := p.CPUPercent()
		if err != nil {
			continue
		}

		name, _ := p.Name()
		user, err := p.Username()
		if err != nil || user == "" {
			user = "?"
		}

		list = append(list, ProcCPU{
			PID:  p.Pid,
			User: user,
			Name: name,
			CPU:  cpu,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CPU > list[j].CPU
	})

	if limit > 0 && limit < len(list) {
		list = list[:limit]
	}

	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PID\tUSER\tPROCESS\tCPU%")

	for _, p := range list {
		_, _ = fmt.Fprintf(
			w, "%d\t%s\t%s\t%.2f\n",
			p.PID,
			p.User,
			p.Name,
			p.CPU,
		)
	}
	_ = w.Flush()
	return sb.String()
}

func Top(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		if !utils.UserIsDev(msg.SenderID) {
			return nil
		}
	}
	helpMsg := "Get Host Top Processes\n" +
		"Argument:\n" +
		"Max amount of Processes, if none is specified list all."

	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}

	bMsg, _ := msg.ReplyText("Processing ...")

	var desc string
	var limit int
	if args != "" {
		limit, _ = strconv.Atoi(args)
		desc = args + " "
	}
	OUTPUT := top(limit)
	caption := fmt.Sprintf("Top %sProcesses", desc)
	// If output is long, upload as a file and send as document
	if len(OUTPUT) > maxLen {
		_, _ = msg.UploadDoc(&tele.UploadOptions{
			Bytes:   []byte(OUTPUT),
			Name:    "top.txt",
			Caption: caption,
			Quote:   true,
		})
		_ = bMsg.Delete()
		return nil
	}

	text := fmt.Sprintf("<pre language=\"%s\">%s</pre>", caption, OUTPUT)
	_, _ = bMsg.Edit(text, &tele.EditOptions{
		HTMLParseMode: true,
	})
	return nil
}
