package startup

import (
	"github.com/celestix/gotgproto/dispatcher"
	onevent "github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	//"github.com/gotd/td/tg"
	
	"enc/internal/clients/telegram"
	"enc/workers/handlers"
)


func AddAllHandlers() {
    dispatcher := telegram.Client.Dispatcher
    AddCmdHandlers(dispatcher)
    AddCBHandlers(dispatcher)
}

func AddCmdHandlers(dispatch dispatcher.Dispatcher) {
	dispatch.AddHandler(onevent.NewCommand("start", handlers.Start))
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_"), handlers.ButtonCallback))
}