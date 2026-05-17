package tele

import (
	"github.com/celestix/gotgproto/ext"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/tg"
	"enc/config"
	"enc/internal/clients/telegram"
)

type CallbackQuery struct {
	ChatID   int64
	SenderID int64
	ID       int
	QueryID  string
	Data     string
	IsPrivate bool
	Ctx   *ext.Context
	Query *tg.UpdateBotCallbackQuery
	BotCB *tgbotapi.CallbackQuery
}

func NewQueryFromGotg(q *tg.UpdateBotCallbackQuery, c *ext.Context, chatID int64, senderID int64) *CallbackQuery {
	return &CallbackQuery{Query: q, Ctx: c, ChatID: chatID, SenderID: senderID, ID: q.MsgID, Data: string(q.Data), IsPrivate: chatID == senderID}
}

func NewQueryFromBotAPI(q *tgbotapi.CallbackQuery) *CallbackQuery {
	msgID := 0; if q.Message != nil { msgID = q.Message.MessageID }
	return &CallbackQuery{BotCB: q, ChatID: q.Message.Chat.ID, SenderID: q.From.ID, ID: msgID, QueryID: q.ID, Data: q.Data, IsPrivate: q.Message.Chat.Type == "private"}
}

func (query *CallbackQuery) Answer(msg string, alert bool) error {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil && query.BotCB != nil {
		_, err := telegram.BotAPI.Request(tgbotapi.NewCallback(query.QueryID, msg))
		return err
	}
	_, err := telegram.Context.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{Alert: alert, QueryID: query.Query.QueryID, Message: msg})
	return err
}

func (query *CallbackQuery) GetMessage() *Message {
	if query.BotCB != nil && query.BotCB.Message != nil { return NewMsgFromBotAPI(query.BotCB.Message) }
	ids := &tg.InputMessageCallbackQuery{ID: query.ID, QueryID: query.Query.QueryID}
	msgs, err := GetMessages(query.ChatID, ids)
	if err != nil { return &Message{Ctx: telegram.Context, ChatID: query.ChatID, SenderID: query.SenderID, ID: query.ID} }
	return NewMsgFromGotg(msgs[0], telegram.Context, query.ChatID, query.SenderID)
}
