package tele

import (
	"context"

	botmodels "enc/internal/clients/telegram/botapi/models"
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
	BotAPIID string
}

func NewQueryFromGotg(q *tg.UpdateBotCallbackQuery, c any, chatID int64, senderID int64) *CallbackQuery {
	ctx := unwrapGotgContext(c)
	return &CallbackQuery{
		Query:    q,
		Ctx:      ctx,
		ChatID:   chatID,
		SenderID: senderID,
		ID:       q.MsgID,
	}
}

func NewQueryFromBotAPI(q *botmodels.CallbackQuery) *CallbackQuery {
	if q == nil {
		return nil
	}
	chatID := int64(0)
	msgID := 0
	if q.Message.Message != nil {
		chatID = q.Message.Message.Chat.ID
		msgID = q.Message.Message.ID
	} else if q.Message.InaccessibleMessage != nil {
		chatID = q.Message.InaccessibleMessage.Chat.ID
		msgID = q.Message.InaccessibleMessage.MessageID
	}
	return &CallbackQuery{
		Query: &tg.UpdateBotCallbackQuery{
			MsgID: msgID,
			Data:  []byte(q.Data),
		},
		ChatID:   chatID,
		SenderID: q.From.ID,
		ID:       msgID,
		BotAPIID: q.ID,
	}
}

func (query *CallbackQuery) Answer(msg string, alert bool) error {
	if query.BotAPIID != "" {
		err := telegram.AnswerBotAPICallback(context.Background(), query.BotAPIID, msg, alert)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
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
	if query.BotAPIID != "" {
		return &Message{ChatID: query.ChatID, SenderID: query.SenderID, ID: query.ID}
	}
	ids := &tg.InputMessageCallbackQuery{
		ID:      query.ID,            // message id (int)
		QueryID: query.Query.QueryID, // callback query id (int64)
	}
	msgs, err := GetMessages(query.ChatID, ids)
	if err != nil {
		logger.Warn(err)
		return &Message{
			Ctx:      query.Ctx,
			ChatID:   query.ChatID,
			SenderID: query.SenderID,
			ID:       query.ID,
		}
	}
	return NewMsgFromGotg(
		msgs[0],
		query.Ctx,
		query.ChatID,
		query.SenderID,
	)
}
