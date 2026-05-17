package tele

import (
	"fmt"
	"github.com/gotd/td/tg"
	"enc/internal/clients/telegram"
)

func GetRepliedToMessage(msg *Message) (*Message, error) {
	if msg.ReplyToID == 0 {
		return nil, nil
	}
	if telegram.BotAPI != nil && msg.BotMsg != nil && msg.BotMsg.ReplyToMessage != nil {
		return NewMsgFromBotAPI(msg.BotMsg.ReplyToMessage), nil
	}
	msgs, err := GetMessages(msg.ChatID, &tg.InputMessageID{ID: msg.ReplyToID})
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("message not found")
	}
	return NewMsgFromGotg(msgs[0], telegram.Context, msg.ChatID, GetSenderIDFromMessage(msgs[0])), nil
}
