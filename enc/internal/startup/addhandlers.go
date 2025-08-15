package startup

import (
	//"fmt"

	"github.com/celestix/gotgproto/dispatcher"
	onevent "github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/tg"

	"enc/config"
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
	noArgHandlers := map[string]BaseEventHandler{
		"start":  BaseEventHandler(handlers.Start),
		"help":   BaseEventHandler(handlers.Start),
		"ping":   BaseEventHandler(handlers.Up),
		"status": BaseEventHandler(handlers.Status),
	}

	for cmd, handler := range noArgHandlers {
		dispatch.AddHandler(onevent.NewCommand(
			addSuffixcmd(cmd),
			handler,
		))
	}
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_"), handlers.ButtonCallback))
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("ihelp"), CallbackEventHandler(handlers.Help)))
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("back_cb"), CallbackEventHandler(handlers.StartBack)))
	dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix("icommands"), CallbackEventHandler(handlers.Commands)))
}

func addSuffixcmd(cmd string) string {
	return cmd + config.Conf.CmdSuffix
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
			Ctx:      ctx,
			ChatID:   chat.GetID(),
			SenderID: user.ID,
			ID:       update.CallbackQuery.MsgID,
		}
		err := handler(ctx, update, msg)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}
