package tele

import (
	"context"
	"strings"

	botmodels "enc/internal/clients/telegram/botapi/models"
	"github.com/celestix/gotgproto/ext"
	gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/tg"
)

// Backend identifies the Telegram transport that produced an event.
type Backend string

const (
	BackendGotg   Backend = "gotgproto"
	BackendBotAPI Backend = "bot-api"
)

// Context is the handler-facing Telegram context. It embeds the gotgproto
// context when the MTProto backend is active and carries the Bot API client
// context when the HTTP Bot API backend is active.
type Context struct {
	*ext.Context
	StdContext context.Context
	Backend    Backend
	BotAPI     any
}

// Update is the handler-facing Telegram update. It exposes the message and
// callback fields handlers already use while retaining the originating backend.
type Update struct {
	*ext.Update
	Backend          Backend
	BotAPI           any
	EffectiveMessage *gp.Message
	CallbackQuery    *tg.UpdateBotCallbackQuery
	args             []string
	chatID           int64
	senderID         int64
}

// NewGotgContext wraps a gotgproto context for transport-neutral handlers.
func NewGotgContext(ctx *ext.Context) *Context {
	return &Context{Context: ctx, Backend: BackendGotg}
}

// NewGotgUpdate wraps a gotgproto update for transport-neutral handlers.
func NewGotgUpdate(update *ext.Update) *Update {
	return &Update{
		Update:           update,
		Backend:          BackendGotg,
		EffectiveMessage: update.EffectiveMessage,
		CallbackQuery:    update.CallbackQuery,
	}
}

// NewBotAPIContext wraps a Bot API context/client value for transport-neutral handlers.
func NewBotAPIContext(ctx context.Context, botAPI any) *Context {
	return &Context{StdContext: ctx, Backend: BackendBotAPI, BotAPI: botAPI}
}

// NewBotAPIUpdate wraps a Bot API update value for transport-neutral handlers.
func NewBotAPIUpdate(update *botmodels.Update) *Update {
	u := &Update{Backend: BackendBotAPI, BotAPI: update}
	if update == nil {
		return u
	}
	if update.Message != nil {
		u.EffectiveMessage = NewGotgMessageFromBotAPI(update.Message)
		u.args = strings.Fields(update.Message.Text)
		u.chatID = update.Message.Chat.ID
		if update.Message.From != nil {
			u.senderID = update.Message.From.ID
		}
		return u
	}
	if update.CallbackQuery != nil {
		query := update.CallbackQuery
		if query.Message.Message != nil {
			u.EffectiveMessage = NewGotgMessageFromBotAPI(query.Message.Message)
			u.chatID = query.Message.Message.Chat.ID
		} else if query.Message.InaccessibleMessage != nil {
			u.chatID = query.Message.InaccessibleMessage.Chat.ID
			u.EffectiveMessage = &gp.Message{Message: &tg.Message{ID: query.Message.InaccessibleMessage.MessageID}}
		}
		u.senderID = query.From.ID
		u.CallbackQuery = &tg.UpdateBotCallbackQuery{
			MsgID: queryMessageID(query),
			Data:  []byte(query.Data),
		}
	}
	return u
}

func (u *Update) Args() []string {
	if u == nil {
		return nil
	}
	if u.Backend == BackendBotAPI {
		return u.args
	}
	if u.Update == nil {
		return nil
	}
	return u.Update.Args()
}

func (u *Update) EffectiveChat() gp.EffectiveChat {
	if u == nil {
		return &simpleEffectiveChat{}
	}
	if u.Backend == BackendGotg && u.Update != nil {
		return u.Update.EffectiveChat()
	}
	return &simpleEffectiveChat{id: u.chatID}
}

func (u *Update) EffectiveUser() *tg.User {
	if u == nil {
		return nil
	}
	if u.Backend == BackendGotg && u.Update != nil {
		return u.Update.EffectiveUser()
	}
	return &tg.User{ID: u.senderID}
}

func unwrapGotgContext(ctx any) *ext.Context {
	switch c := ctx.(type) {
	case *ext.Context:
		return c
	case *Context:
		if c == nil {
			return nil
		}
		return c.Context
	default:
		return nil
	}
}

func queryMessageID(query *botmodels.CallbackQuery) int {
	if query == nil {
		return 0
	}
	if query.Message.Message != nil {
		return query.Message.Message.ID
	}
	if query.Message.InaccessibleMessage != nil {
		return query.Message.InaccessibleMessage.MessageID
	}
	return 0
}

type simpleEffectiveChat struct{ id int64 }

func (c *simpleEffectiveChat) GetID() int64                          { return c.id }
func (c *simpleEffectiveChat) GetAccessHash() int64                  { return 0 }
func (c *simpleEffectiveChat) IsAChannel() bool                      { return false }
func (c *simpleEffectiveChat) IsAChat() bool                         { return true }
func (c *simpleEffectiveChat) IsAUser() bool                         { return false }
func (c *simpleEffectiveChat) GetInputUser() tg.InputUserClass       { return nil }
func (c *simpleEffectiveChat) GetInputChannel() tg.InputChannelClass { return nil }
func (c *simpleEffectiveChat) GetInputPeer() tg.InputPeerClass {
	return &tg.InputPeerChat{ChatID: c.id}
}
