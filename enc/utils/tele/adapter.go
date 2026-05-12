package tele

import (
	"strconv"
	"strings"

	"enc/internal/clients/telegram"
	gp_ext "github.com/celestix/gotgproto/ext"
	gp_types "github.com/celestix/gotgproto/types"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/constant"
	"github.com/gotd/td/tg"
)

func BotUpdateToGpUpdate(u tgbotapi.Update) (*gp_ext.Update, *Message) {
	gpUpdate := &gp_ext.Update{
		Entities: &tg.Entities{
			Users:    make(map[int64]*tg.User),
			Chats:    make(map[int64]*tg.Chat),
			Channels: make(map[int64]*tg.Channel),
		},
	}
	var msg *Message

	if u.Message != nil {
		var peer tg.PeerClass
		chat := u.Message.Chat
		if chat.Type == "private" {
			peer = &tg.PeerUser{UserID: chat.ID}
		} else if chat.Type == "group" {
			peer = &tg.PeerChat{ChatID: -chat.ID}
		} else {
			// supergroup or channel
			// Telegram Bot API group IDs are -100xxxxxxxxxx
			// MTProto uses the positive part
			idStr := strconv.FormatInt(chat.ID, 10)
			if strings.HasPrefix(idStr, "-100") {
				id, _ := strconv.ParseInt(idStr[4:], 10, 64)
				peer = &tg.PeerChannel{ChannelID: id}
			} else {
				peer = &tg.PeerChat{ChatID: -chat.ID}
			}
		}

		gpMsg := &gp_types.Message{
			Message: &tg.Message{
				ID:      u.Message.MessageID,
				Message: u.Message.Text,
				PeerID:  peer,
				FromID:  &tg.PeerUser{UserID: u.Message.From.ID},
			},
			Text: u.Message.Text,
		}
		if u.Message.ReplyToMessage != nil {
			gpMsg.ReplyTo = &tg.MessageReplyHeader{
				ReplyToMsgID: u.Message.ReplyToMessage.MessageID,
			}
		}

		gpUpdate.EffectiveMessage = gpMsg
		gpUpdate.UpdateClass = &tg.UpdateNewMessage{
			Message: gpMsg.Message,
		}

		msg = &Message{
			ChatID:   u.Message.Chat.ID,
			ID:       u.Message.MessageID,
			SenderID: u.Message.From.ID,
			Raw:      gpMsg,
			Ctx:      telegram.Context,
		}
		if u.Message.ReplyToMessage != nil {
			msg.ReplyToID = u.Message.ReplyToMessage.MessageID
		}
	} else if u.CallbackQuery != nil {
		var peer tg.PeerClass
		chat := u.CallbackQuery.Message.Chat
		if chat.Type == "private" {
			peer = &tg.PeerUser{UserID: chat.ID}
		} else if chat.Type == "group" {
			peer = &tg.PeerChat{ChatID: -chat.ID}
		} else {
			idStr := strconv.FormatInt(chat.ID, 10)
			if strings.HasPrefix(idStr, "-100") {
				id, _ := strconv.ParseInt(idStr[4:], 10, 64)
				peer = &tg.PeerChannel{ChannelID: id}
			} else {
				peer = &tg.PeerChat{ChatID: -chat.ID}
			}
		}

		var qid int64
		if id, err := strconv.ParseInt(u.CallbackQuery.ID, 10, 64); err == nil {
			qid = id
		}

		gpUpdate.CallbackQuery = &tg.UpdateBotCallbackQuery{
			QueryID: qid,
			Data:    []byte(u.CallbackQuery.Data),
			MsgID:   u.CallbackQuery.Message.MessageID,
			Peer:    peer,
		}

		gpUpdate.UpdateClass = gpUpdate.CallbackQuery

		msg = &Message{
			ChatID:   u.CallbackQuery.Message.Chat.ID,
			ID:       u.CallbackQuery.Message.MessageID,
			SenderID: u.CallbackQuery.From.ID,
			Ctx:      telegram.Context,
			Raw: &gp_types.Message{
				Message: &tg.Message{
					ID:      u.CallbackQuery.Message.MessageID,
					Message: u.CallbackQuery.Message.Text,
					PeerID:  peer,
					FromID:  &tg.PeerUser{UserID: u.CallbackQuery.From.ID},
				},
				Text: u.CallbackQuery.Message.Text,
			},
		}
	}

	return gpUpdate, msg
}

func (m *Message) GetArgs() []string {
	if m.Raw != nil {
		return strings.Fields(m.Raw.Text)
	}
	return nil
}

type botAPIChat struct {
	id int64
}

func (c *botAPIChat) GetID() int64         { return c.id }
func (c *botAPIChat) GetAccessHash() int64 { return 0 }
func (c *botAPIChat) IsAChannel() bool     { return constant.TDLibPeerID(c.id).IsChannel() }
func (c *botAPIChat) IsAChat() bool        { return constant.TDLibPeerID(c.id).IsChat() }
func (c *botAPIChat) IsAUser() bool        { return constant.TDLibPeerID(c.id).IsUser() }
func (c *botAPIChat) GetInputUser() tg.InputUserClass {
	return &tg.InputUser{UserID: c.id}
}

func (c *botAPIChat) GetInputChannel() tg.InputChannelClass {
	return &tg.InputChannel{ChannelID: constant.TDLibPeerID(c.id).ToPlain()}
}

func (c *botAPIChat) GetInputPeer() tg.InputPeerClass {
	return &tg.InputPeerUser{UserID: c.id} // simplified
}
