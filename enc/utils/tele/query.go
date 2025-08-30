package tele

import (
	"fmt"
	"path/filepath"

	"github.com/celestix/gotgproto/ext"
	gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
	"enc/utils/tele/parser"
	"enc/utils/tele/parser/htmlparser"
)

type CallbackQuery struct {
	ChatID    int64
	SenderID  int64
	ID        int
	Ctx       *ext.Context
	Query     *tg.UpdateBotCallbackQuery
	Chat      *tg.Chat
	User      *tg.User
}

func NewQueryFromGotg(q *tg.UpdateBotCallbackQuery, c *ext.Context, chatID int64, senderID int64) *Message {
	return &CallbackQuery{
		Query:      q,
		Ctx:       c,
		ChatID:    chatID,
		SenderID:  senderID,
		ID:        q.MsgID,
	}
}

func (query *CallbackQuery) Answer(msg string, alert bool) error {
	_, err := query.Ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
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
	return &Message{
		Ctx:      query.Ctx,
		ChatID:   query.ChatID,
		SenderID: query.SenderID,
		ID:       query.ID,
	}
}