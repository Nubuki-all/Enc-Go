package manage

import (
	"enc/utils"
	"enc/utils/tele"

)

func EditChannelMsg(msg *tele.Message, args string) error {
	if !utils.UserIsOwner(msg.SenderID) {
		return nil
	}
	helpMsg := "Edit channel media with a replied Message.\n" +
		"Argument: link of message to edit."
	if args == "" || msg.ReplyToID == 0 {
		args = "--help"
	}
	if utils.SendHelp(args) {
		_, _ = msg.ReplyText(utils.CodeBlock(helpMsg, false))
		return nil
	}

	err := msg.SetRepliedToMessage(); repMsg := msg.ReplyToMessage
	if err != nil {
		_, _ = msg.ReplyText(err.Error())
		return nil
	}
	if repMsg == nil {
		_, _ = msg.ReplyText("No replied message.")
		return nil
	}
	if repMsg.Text == "" && repMsg.Media == nil {
		_, _ = msg.ReplyText("Invalid replied message.")
		return nil
	}
	oMsg := tele.GetMessageFromLink(args)
	if oMsg == nil {
		_, _ = msg.ReplyText("Could not retrieve message from the link.")
		return nil
	}
	media, _ := utils.ToInputMedia(repMsg.Media)
	_, _ = oMsg.Edit(repMsg.Text, &tele.EditOptions{Entities: repMsg.Entities, Media: media})
	_, _ = msg.ReplyText("Edited succesfully.")
	return nil
}
