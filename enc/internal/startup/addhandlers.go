package startup

import (
	//"fmt"

	"github.com/celestix/gotgproto/dispatcher"
	onevent "github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/tg"

	"enc/internal/clients/telegram"
	"enc/internal/logger"
	"enc/utils/tele"
	"enc/workers/handlers"
)

func AddAllHandlers() {
	dispatcher := telegram.Client.Dispatcher
	AddCmdHandlers(dispatcher)
	AddCBHandlers(dispatcher)
}

func AddCmdHandlers(dispatch dispatcher.Dispatcher) {
	dispatch.AddHandler(onevent.NewCommand("start1", handlers.StartEx))
	dispatch.AddHandler(onevent.NewCommand("start", BaseEventHandler(handlers.Start)))
	dispatch.AddHandler(onevent.NewCommand("help", BaseEventHandler(handlers.Start)))
	dispatch.AddHandler(onevent.NewCommand("ping", BaseEventHandler(handlers.Up)))
	dispatch.AddHandler(onevent.NewCommand("status", BaseEventHandler(handlers.Status)))
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_"), handlers.ButtonCallback))
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("ihelp"), CallbackEventHandler(handlers.Help)))
}

func BaseEventHandler(handler func(*ext.Context, *ext.Update, *tele.Message) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		user := update.EffectiveUser()
		chat := update.EffectiveChat()
		msg := tele.NewMsgFromGotg(
			update.EffectiveMessage,
			ctx,
			chat.GetID(),
			user.ID,
		)
		err := handler(ctx, update, msg)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}


func CallbackEventHandler(handler func(*ext.Context, *ext.Update, *tele.Message) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		user := update.EffectiveUser()
		chat := update.EffectiveChat()
		msg := &tele.Message{
			Ctx: ctx,
			ChatID: chat.GetID(),
			SenderID: user.ID,
			ID: update.CallbackQuery.MsgID,
		}
		err := handler(ctx, update, msg)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}
