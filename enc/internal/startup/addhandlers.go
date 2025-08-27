package startup

import (
	//"fmt"
	"strings"

	"github.com/celestix/gotgproto/dispatcher"
	onevent "github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	//"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
	"enc/utils/tele"
	"enc/utils/tele/parser"
	"enc/workers/handlers"
	"enc/workers/handlers/dev"
	"enc/workers/handlers/manage"
)

func AddAllHandlers() {
	dispatcher := telegram.Client.Dispatcher
	AddCmdHandlers(dispatcher)
	AddCBHandlers(dispatcher)

	dispatcher.AddHandlerToGroup(onevent.NewMessage(filters.Message.Text, handlers.Print), 1)
}

func AddCmdHandlers(dispatch dispatcher.Dispatcher) {
	cmdHandlers := map[string]func(*ext.Context, *ext.Update) error{
		// Welcome Handlers
		"start": BaseEventHandler(handlers.Start),
		"help":  BaseEventHandler(handlers.Start),

		// Status Handlers
		"ping":   BaseEventHandler(handlers.Up),
		"status": BaseEventHandler(handlers.Status),

		// Permit Handlers
		"unpermit": EventHandlerWithArgs(manage.TempUnauth),
		"permit":   EventHandlerWithArgs(manage.TempAuth),

		// Filter-related Handlers
		"setrename": EventHandlerWithArgs(manage.CustomRename),
		"name":      EventHandlerWithArgs(manage.AutoRename),
		"vname":     EventHandlerWithArgs(manage.VAutoRename),
		"delname":   EventHandlerWithArgs(manage.DelAutoRename),
		"filter":    EventHandlerWithArgs(manage.Filter),
		"vfilter":   EventHandlerWithArgs(manage.VFilter),
		"delfilter": EventHandlerWithArgs(manage.DelFilter),
		"mtag":      EventHandlerWithArgs(manage.MatchFilter),
		"etag":      EventHandlerWithArgs(manage.ManageMatchFilter),
		"xtag":      EventHandlerWithArgs(manage.ExtCapFilter),
		"extag":     EventHandlerWithArgs(manage.ManageExtCapFilter),

		// Encode parameter-related Handlers
		"set":   EventHandlerWithArgs(manage.ChangeEParams),
		"get":   EventHandlerWithArgs(manage.CheckEParams),
		"reset": EventHandlerWithArgs(manage.ResetEParams),
		"mset":  EventHandlerWithArgs(manage.SetMuxArgs),
		"mget":  EventHandlerWithArgs(manage.GetMuxArgs),
		"v":     EventHandlerWithArgs(manage.Version),
		"parse": EventHandlerWithArgs(manage.MGlobalPOptions),
		"pause": EventHandlerWithArgs(manage.Pause),

		// Forward Handler
		"forward": EventHandlerWithArgs(manage.FCForward),

		// Run-control Handlers
		"restart": EventHandlerWithArgs(manage.Restart),

		// Dev Handlers
		"eval": EventHandlerWithRawArgs(dev.Eval),
		"bash": EventHandlerWithRawArgs(dev.Bash),
	}

	for cmd, handler := range cmdHandlers {
		dispatch.AddHandler(onevent.NewCommand(
			addSuffixcmd(cmd),
			handler,
		))
	}
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
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

func EventHandlerWithArgs(handler func(*ext.Context, *ext.Update, *tele.Message, string) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		user := update.EffectiveUser()
		chat := update.EffectiveChat()
		_, args, _ := strings.Cut(update.EffectiveMessage.Text, " ")
		args = strings.Trim(args, " ")
		msg := tele.NewMsgFromGotg(
			update.EffectiveMessage,
			ctx,
			chat.GetID(),
			user.ID,
		)
		err := handler(ctx, update, msg, args)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}

func EventHandlerWithRawArgs(handler func(*ext.Context, *ext.Update, *tele.Message, string) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		user := update.EffectiveUser()
		chat := update.EffectiveChat()
		text := update.EffectiveMessage.Text
		rawText, err := parser.Unparse(
			text,
			update.EffectiveMessage.Entities,
		)
		if err != nil {
			rawText = text
			logger.Warn(err)
		}
		_, args, _ := strings.Cut(rawText, " ")
		args = strings.Trim(args, " ")
		msg := tele.NewMsgFromGotg(
			update.EffectiveMessage,
			ctx,
			chat.GetID(),
			user.ID,
		)
		err = handler(ctx, update, msg, args)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}
