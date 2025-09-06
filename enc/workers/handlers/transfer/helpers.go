package tranfer

import (
	"fmt"
	
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
	
	"enc/utils"
	"enc/utils/tele"
	"enc/workers/downloaders"
)

func ListFiles(ctx *ext.Context, update *ext.Update, msg *tele.Message, args string) error  {
	// validate argument
	if !utils.IsURL(link) && !utils.IsMagnet(link) {
		_, err := msg.ReplyText("Please pass a valid direct/torrent/magnetic link as argument.")
		return err
	}
	dlInfo := downloaders.GetQbitDlInfo(args)
	if dlInfo.Error != nil {
		_, _ := msg.ReplyText(fmt.Sprintf("`%v`", dlInfo.Error))
		return nil
	}
	var sb strings.Builder
	for i, item := range dlInfo.Files {
		fmt.Fprintf(&sb, "%d. `%s`\n", i, item)
	}
	fmt.Fprintf(&sb, "\n**Total files:** %d\n\n__(While selecting with aria2 subtract 1 to get the file id)__", dlInfo.Count)
	text := sb.String()
	parts := utils.SplitText(msg, "\n", false)
	prevMsg := msg
	for _, part := range parts {
		prevMsg, _ = prevMsg.ReplyText(part)
	}
	return nil
}
