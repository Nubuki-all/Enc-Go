package tele

import (
	"github.com/celestix/gotgproto/ext"
	gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/tg"

	//"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/utils/tele/parser"
)

type Message struct {
	ChatID   int64
	Ctx      *ext.Context
	ID       int
	Raw      *gp.Message
	SenderID int64
	Chat     *tg.Chat
	User     *tg.User
}

type EditOptions struct {
	ReplyMarkup tg.ReplyMarkupClass
	// will add more fields as required
}

type ReplyOptions struct {
	ReplyMarkup tg.ReplyMarkupClass
	// will add more fields as required
}

// NewFromGotg constructs a wrapper from a concrete *gp.Message.
func NewMsgFromGotg(m *gp.Message, c *ext.Context, chatID int64, senderID int64) *Message {
	return &Message{
		Raw:      m,
		Ctx:      c,
		ChatID:   chatID,
		SenderID: senderID,
		ID:       m.Message.ID,
	}
}

// RawMessage returns the underlying *gp.Message.
func (m *Message) RawMessage() *gp.Message {
	return m.Raw
}

func (m *Message) ReplyText(text string) (*Message, error) {
	pText, entities, _ := parser.Parse(text, true)
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: m.ID,
	}
	msg, err := m.Ctx.SendMessage(m.ChatID, &tg.MessagesSendMessageRequest{
		Message:  pText,
		Entities: entities,
		ReplyTo:  inputRep,
	},
	)
	if err != nil {
		return nil, err
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
}

func (m *Message) Reply(text string, replyOptions *ReplyOptions) (*Message, error) {
	pText, entities, _ := parser.Parse(text, true)
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: m.ID,
	}
	msg, err := m.Ctx.SendMessage(m.ChatID, &tg.MessagesSendMessageRequest{
		Message:     pText,
		Entities:    entities,
		ReplyTo:     inputRep,
		ReplyMarkup: replyOptions.ReplyMarkup,
	},
	)
	if err != nil {
		return nil, err
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
}

func (m *Message) EditText(text string) (*Message, error) {
	pText, entities, _ := parser.Parse(text, true)
	msg, err := m.Ctx.EditMessage(m.ChatID, &tg.MessagesEditMessageRequest{
		ID:       m.ID,
		Message:  pText,
		Entities: entities,
	},
	)
	if err != nil {
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.SenderID), nil
}

func (m *Message) Edit(text string, editOptions *EditOptions) (*Message, error) {
	pText, entities, _ := parser.Parse(text, true)
	msg, err := m.Ctx.EditMessage(m.ChatID, &tg.MessagesEditMessageRequest{
		ID:          m.ID,
		Message:     pText,
		Entities:    entities,
		ReplyMarkup: editOptions.ReplyMarkup,
	},
	)
	if err != nil {
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.SenderID), nil
}

func SendMessage(id int64, text string) error {
	peer, _ := GetPeer(id)
	pText, entities, _ := parser.Parse(text, true)
	// ParseMarkdown(text, true)
	_, err := telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{
		Message:  pText,
		Peer:     peer,
		Entities: entities,
	},
	)
	return err
}

func EditMessage(id int64, msgID int, text string) error {
	peer, _ := GetPeer(id)
	pText, entities, _ := parser.Parse(text, true)
	// ParseMarkdown(text, true)
	_, err := telegram.Context.EditMessage(id, &tg.MessagesEditMessageRequest{
		ID:       msgID,
		Message:  pText,
		Peer:     peer,
		Entities: entities,
	},
	)
	return err
}
