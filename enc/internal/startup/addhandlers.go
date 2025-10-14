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
	batchutils "enc/utils/batch"
	queueutils "enc/utils/queue"
	"enc/utils/tele"
	"enc/utils/tele/parser"
	"enc/workers/downloaders"
	"enc/workers/encoders"
	"enc/workers/handlers"
	"enc/workers/handlers/callbacks"
	"enc/workers/handlers/dev"
	"enc/workers/handlers/manage"
	"enc/workers/handlers/queue"
	"enc/workers/handlers/transfer"
	"enc/workers/uploaders"
)

func AddAllHandlers() {
	d := telegram.Client.Dispatcher
	AddCmdHandlers(d)
	AddCBHandlers(d)

	// dispatcher.AddHandlerToGroup(onevent.NewMessage(filters.Message.Text, handlers.Print), 1)
	d.AddHandler(onevent.NewMessage(filters.Message.Text, handlers.Print))
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
		"bash": EventHandlerWithRawArgs(dev.Bash),
		"eval": EventHandlerWithRawArgs(dev.Eval),
		"logs": EventHandlerWithArgs(dev.GetLogs),

		// Transfer Handlers
		"download": EventHandlerWithArgs(transfer.Download),
		"dl":       EventHandlerWithArgs(transfer.Download),
		"rename":   EventHandlerWithArgs(transfer.Rename),
		"list":     EventHandlerWithArgs(transfer.ListFiles),
		"listv":    EventHandlerWithArgs(transfer.ListYtdlFiles),
		"mux":      EventHandlerWithArgs(transfer.Mux),
		"upload":   EventHandlerWithArgs(transfer.Upload),
		"ul":       EventHandlerWithArgs(transfer.Upload),

		// Queue-related Handlers
		// Add:
		"l":  EventHandlerWithArgs(queue.AddLink),
		"ql": EventHandlerWithArgs(queue.AddTorrent),
		// Manage
		"queue":  EventHandlerWithArgs(queue.ListQueue),
		"batch":  EventHandlerWithArgs(queue.EditBatch),
		"clear":  EventHandlerWithArgs(queue.ClearQueue),
		"editq":  EventHandlerWithArgs(queue.EditQueue),
		"select": EventHandlerWithArgs(queue.Select),
	}

	for cmd, handler := range cmdHandlers {
		dispatch.AddHandler(onevent.NewCommand(
			addSuffixcmd(cmd),
			handler,
		))
	}
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
	callbackHandlers := map[string]func(*ext.Context, *ext.Update) error{
		// Welcome Handlers
		"ihelp":     CallbackEventHandler(handlers.Help),
		"back_cb":   CallbackEventHandler(handlers.StartBack),
		"icommands": CallbackEventHandler(handlers.Commands),

		// Tranfer Handlers
		downloaders.CallbackData: CallbackEventHandler(callbacks.CancelDownload),
		uploaders.CallbackData:   CallbackEventHandler(callbacks.CancelUpload),

		// Encode Handlers
		encoders.CallbackData:             CallbackEventHandler(callbacks.EncodeInfo),
		encoders.SCallbackData:            CallbackEventHandler(callbacks.EncodeStats),
		encoders.CancelCallbackData:       CallbackEventHandler(callbacks.CancelEncode),
		"j" + encoders.CancelCallbackData: CallbackEventHandler(callbacks.CancelEncodeJobs),

		// Turn Handlers
		"cancel_rturn": CallbackEventHandler(callbacks.CancelRTurn),
		"cancel_mturn": CallbackEventHandler(callbacks.CancelMTurn),

		// Batch Preview Handlers
		batchutils.CallbackData: CallbackEventHandler(batchutils.PreviewAction),

		// Queue Preview Handlers
		queueutils.CallbackData: CallbackEventHandler(queueutils.PreviewAction),
	}
	for prefix, handler := range callbackHandlers {
		dispatch.AddHandler(onevent.NewCallbackQuery(
			filters.CallbackQuery.Prefix(prefix),
			handler,
		))
	}
}

func addSuffixcmd(cmd string) string {
	return cmd + config.Conf.CmdSuffix
}

func BaseEventHandler(handler func(*ext.Context, *ext.Update, *tele.Message) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		// user := update.EffectiveUser()
		chat := update.EffectiveChat()
		msg := tele.NewMsgFromGotg(
			update.EffectiveMessage,
			ctx,
			chat.GetID(),
			// user.ID,
			tele.GetSenderIDFromMessage(update.EffectiveMessage),
		)
		err := handler(ctx, update, msg)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}

func CallbackEventHandler(handler func(*ext.Context, *ext.Update, *tele.CallbackQuery) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		user := update.EffectiveUser()
		chat := update.EffectiveChat()
		query := tele.NewQueryFromGotg(
			update.CallbackQuery,
			ctx,
			chat.GetID(),
			user.ID,
		)
		err := handler(ctx, update, query)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}

func EventHandlerWithArgs(handler func(*ext.Context, *ext.Update, *tele.Message, string) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		// user := update.EffectiveUser()
		chat := update.EffectiveChat()
		_, args, _ := strings.Cut(update.EffectiveMessage.Text, " ")
		args = strings.Trim(args, " ")
		msg := tele.NewMsgFromGotg(
			update.EffectiveMessage,
			ctx,
			chat.GetID(),
			// user.ID,
			tele.GetSenderIDFromMessage(update.EffectiveMessage),
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
		// user := update.EffectiveUser()
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
			// user.ID,
			tele.GetSenderIDFromMessage(update.EffectiveMessage),
		)
		err = handler(ctx, update, msg, args)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}
