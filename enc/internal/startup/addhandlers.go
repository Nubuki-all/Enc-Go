package startup

import (
	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	onevent "github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	//"github.com/gotd/td/tg"
	
	"enc/internal/clients/telegram"
	"enc/utils/tele"
	"enc/workers/handlers"
)


func AddAllHandlers() {
    dispatcher := telegram.Client.Dispatcher
    AddCmdHandlers(dispatcher)
    AddCBHandlers(dispatcher)
}

func AddCmdHandlers(dispatch dispatcher.Dispatcher) {
	dispatch.AddHandler(onevent.NewCommand("start", handlers.Start))
	dispatch.AddHandler(onevent.NewCommand("ping", BaseEventHandler(handlers.Up)))
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_"), handlers.ButtonCallback))
}




func BaseEventHandler(handler func(*ext.Context, *ext.Update, *tele.Message) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		user := update.EffectiveUser()
		chat := update.GetChat()
		msg := tele.NewMsgFromGotg(
			update.EffectiveMessage,
			ctx,
			chat.ID,
			user.ID,
		)
		return handler(ctx, update, msg) 
	}
}
