package tele

import (
	"github.com/celestix/gotgproto/ext"
	// gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/tg"

	//"enc/config"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
)

type CallbackQuery struct {
	ChatID   int64
	SenderID int64
	ID       int
	Ctx      *ext.Context
	Query    *tg.UpdateBotCallbackQuery
	Chat     *tg.Chat
	User     *tg.User
}

func NewQueryFromGotg(q *tg.UpdateBotCallbackQuery, c *ext.Context, chatID int64, senderID int64) *CallbackQuery {
	return &CallbackQuery{
		Query:    q,
		Ctx:      telegram.Context,
		ChatID:   chatID,
		SenderID: senderID,
		ID:       q.MsgID,
	}
}

func (query *CallbackQuery) Answer(msg string, alert bool) error {
	_, err := telegram.Context.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		Alert:   alert,
		QueryID: query.Query.QueryID,
		Message: msg,
	})
	if err != nil {
		logger.ErrorW(err.Error())
	}
	return err
}

func (query *CallbackQuery) GetMessage() *Message {
	ids := &tg.InputMessageCallbackQuery{
		ID:      query.ID,            // message id (int)
		QueryID: query.Query.QueryID, // callback query id (int64)
	}
	msgs, err := GetMessages(query.ChatID, ids)
	if err != nil {
		logger.Warn(err)
		return &Message{
			Ctx:      telegram.Context,
			ChatID:   query.ChatID,
			SenderID: query.SenderID,
			ID:       query.ID,
		}
	}
	return NewMsgFromGotg(
		msgs[0],
		telegram.Context,
		query.ChatID,
		query.SenderID,
	)
}
