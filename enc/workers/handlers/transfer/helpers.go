package transfer

import (
	"fmt"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"

	"enc/utils"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func ListFiles(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error {
	helpMsg := "List items in a torrent along with their file_ids\n" +
		"Arguments:\n" +
		"   direct/torrent/magnetic link (Required)"
	if utils.SendHelp(args) {
		_, _ = msg.Reply("<pre>"+helpMsg+"</pre>",
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	if !utils.IsURL(args) && !utils.IsMagnet(args) {
		_, err := msg.ReplyText("Please pass a valid direct/torrent/magnetic link as argument.")
		return err
	}
	var dlInfo *utils.DLInfo
	if utils.IsMegaLink(args) {
		dlInfo = downloaders.GetMegaDlInfo(args)
	} else {
		dlInfo = downloaders.GetQbitDlInfo(args)
	}
	if dlInfo.Error != nil {
		_, _ = msg.Reply(
			fmt.Sprintf("<pre>%v</pre>", dlInfo.Error),
			&tele.ReplyOptions{HtmlParseMode: true},
		)
		return nil
	}
	var sb strings.Builder
	for i, item := range dlInfo.Files {
		fmt.Fprintf(&sb, "%d. `%s`\n", i, item)
	}
	fmt.Fprintf(&sb, "\n**Total files:** %d\n\n__(While selecting with aria2 add 1 to the file id)__", dlInfo.Count)
	text := sb.String()
	parts := utils.SplitText(text, "\n", false)
	prevMsg := msg
	for _, part := range parts {
		prevMsg, _ = prevMsg.ReplyText(part)
		time.Sleep(1 * time.Second)
	}
	return nil
}
