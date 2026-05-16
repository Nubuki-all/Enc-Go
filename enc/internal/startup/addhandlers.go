package startup

import (
	//"fmt"
	"context"
	"regexp"
	"strings"

	botapi "enc/internal/clients/telegram/botapi"
	"enc/internal/clients/telegram/botapi/models"
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
	"enc/workers/handlers/info"
	"enc/workers/handlers/manage"
	"enc/workers/handlers/queue"
	"enc/workers/handlers/transfer"
	"enc/workers/uploaders"
)

func AddAllHandlers() {
	if config.Conf.TgDispatcher.UsesGotg() {
		if telegram.Client == nil {
			logger.WarnW("gotgproto handler registration skipped: client is not initialized")
			return
		}
		d := telegram.Client.Dispatcher
		AddCmdHandlers(d)
		AddCBHandlers(d)
	}
	if config.Conf.TgDispatcher.UsesBotAPI() {
		AddBotAPIHandlers(telegram.BotAPI)
	}

	// dispatcher.AddHandlerToGroup(onevent.NewMessage(filters.Message.Text, handlers.Print), 1)
	// d.AddHandler(onevent.NewMessage(filters.Message.Text, handlers.Print))
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

		// Edit-Media Handler
		"editmedia": EventHandlerWithArgs(manage.EditChannelMsg),

		// Run-control Handlers
		"restart": EventHandlerWithArgs(manage.Restart),
		"nuke":    EventHandlerWithArgs(manage.Nuke),

		// Rss Handlers
		"rss": EventHandlerWithArgs(manage.RssHandler),

		// Dev Handlers
		"bash": EventHandlerWithRawArgs(dev.Bash),
		"eval": EventHandlerWithRawArgs(dev.Eval),
		"logs": EventHandlerWithArgs(dev.GetLogs),
		"sh":   EventHandlerWithArgs(dev.Sh),
		"top":  EventHandlerWithArgs(dev.Top),

		// Transfer Handlers
		"download": EventHandlerWithArgs(transfer.Download),
		"dl":       EventHandlerWithArgs(transfer.Download),
		"rename":   EventHandlerWithArgs(transfer.Rename),
		"list":     EventHandlerWithArgs(transfer.ListFiles),
		"listv":    EventHandlerWithArgs(transfer.ListYtdlFiles),
		"m":        EventHandlerWithArgs(transfer.MediaInfo),
		"mux":      EventHandlerWithArgs(transfer.Mux),
		"upload":   EventHandlerWithArgs(transfer.Upload),
		"ul":       EventHandlerWithArgs(transfer.Upload),

		// Queue-related Handlers
		// Add:
		"l":   EventHandlerWithArgs(queue.AddLink),
		"ql":  EventHandlerWithArgs(queue.AddTorrent),
		"yl":  EventHandlerWithArgs(queue.AddWithYtdl),
		"add": EventHandlerWithArgs(queue.AddVideo),
		// Manage:
		"queue":     EventHandlerWithArgs(queue.ListQueue),
		"batch":     EventHandlerWithArgs(queue.EditBatch),
		"clear":     EventHandlerWithArgs(queue.ClearQueue),
		"editq":     EventHandlerWithArgs(queue.EditQueue),
		"select":    EventHandlerWithArgs(queue.Select),
		"cancelall": EventHandlerWithArgs(manage.CancelProcesses),

		// Info Handlers:
		"anime":  EventHandlerWithArgs(info.Anime),
		"airing": EventHandlerWithArgs(info.AnimeAiring),
	}

	for cmd, handler := range cmdHandlers {
		dispatch.AddHandler(onevent.NewCommand(
			addSuffixcmd(cmd),
			handler,
		))
	}

	// Media Auto Handler
	dispatch.AddHandler(
		onevent.NewMessage(
			filters.Message.Media,
			BaseEventHandler(queue.AutoAddDocument),
		),
	)
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
	callbackHandlers := map[string]func(*ext.Context, *ext.Update) error{
		// Welcome Handlers
		"ihelp":     CallbackEventHandler(handlers.Help),
		"iback":     CallbackEventHandler(handlers.StartBack),
		"icommands": CallbackEventHandler(handlers.Commands),

		// Tranfer Handlers
		downloaders.CallbackData:  CallbackEventHandler(callbacks.CancelDownload),
		downloaders.CallbackDataB: CallbackEventHandler(callbacks.RevertView),
		downloaders.CallbackDataM: CallbackEventHandler(callbacks.DLStat),
		downloaders.CallbackDataI: CallbackEventHandler(callbacks.ViewDlInfo),
		uploaders.CallbackData:    CallbackEventHandler(callbacks.CancelUpload),

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

func BaseEventHandler(handler func(*tele.Context, *tele.Update, *tele.Message) error) func(*ext.Context, *ext.Update) error {
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
		eventCtx := tele.NewGotgContext(ctx)
		eventUpdate := tele.NewGotgUpdate(update)
		err := handler(eventCtx, eventUpdate, msg)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}

func CallbackEventHandler(handler func(*tele.Context, *tele.Update, *tele.CallbackQuery) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		user := update.EffectiveUser()
		chat := update.EffectiveChat()
		query := tele.NewQueryFromGotg(
			update.CallbackQuery,
			ctx,
			chat.GetID(),
			user.ID,
		)
		eventCtx := tele.NewGotgContext(ctx)
		eventUpdate := tele.NewGotgUpdate(update)
		err := handler(eventCtx, eventUpdate, query)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}

func EventHandlerWithArgs(handler func(*tele.Context, *tele.Update, *tele.Message, string) error) func(*ext.Context, *ext.Update) error {
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
		eventCtx := tele.NewGotgContext(ctx)
		eventUpdate := tele.NewGotgUpdate(update)
		err := handler(eventCtx, eventUpdate, msg, args)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}

func EventHandlerWithRawArgs(handler func(*tele.Context, *tele.Update, *tele.Message, string) error) func(*ext.Context, *ext.Update) error {
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
		eventCtx := tele.NewGotgContext(ctx)
		eventUpdate := tele.NewGotgUpdate(update)
		err = handler(eventCtx, eventUpdate, msg, args)
		if err != nil {
			logger.ErrorW(err.Error())
		}
		return err
	}
}

func AddBotAPIHandlers(b *botapi.Bot) {
	if b == nil {
		logger.WarnW("bot api handler registration skipped: client is not initialized")
		return
	}
	AddBotAPICmdHandlers(b)
	AddBotAPICBHandlers(b)
}

func AddBotAPICmdHandlers(b *botapi.Bot) {
	cmdHandlers := map[string]botapi.HandlerFunc{
		"start": BotAPIBaseEventHandler(handlers.Start),
		"help":  BotAPIBaseEventHandler(handlers.Start),

		"ping":   BotAPIBaseEventHandler(handlers.Up),
		"status": BotAPIBaseEventHandler(handlers.Status),

		"unpermit": BotAPIEventHandlerWithArgs(manage.TempUnauth),
		"permit":   BotAPIEventHandlerWithArgs(manage.TempAuth),

		"setrename": BotAPIEventHandlerWithArgs(manage.CustomRename),
		"name":      BotAPIEventHandlerWithArgs(manage.AutoRename),
		"vname":     BotAPIEventHandlerWithArgs(manage.VAutoRename),
		"delname":   BotAPIEventHandlerWithArgs(manage.DelAutoRename),
		"filter":    BotAPIEventHandlerWithArgs(manage.Filter),
		"vfilter":   BotAPIEventHandlerWithArgs(manage.VFilter),
		"delfilter": BotAPIEventHandlerWithArgs(manage.DelFilter),
		"mtag":      BotAPIEventHandlerWithArgs(manage.MatchFilter),
		"etag":      BotAPIEventHandlerWithArgs(manage.ManageMatchFilter),
		"xtag":      BotAPIEventHandlerWithArgs(manage.ExtCapFilter),
		"extag":     BotAPIEventHandlerWithArgs(manage.ManageExtCapFilter),

		"set":   BotAPIEventHandlerWithArgs(manage.ChangeEParams),
		"get":   BotAPIEventHandlerWithArgs(manage.CheckEParams),
		"reset": BotAPIEventHandlerWithArgs(manage.ResetEParams),
		"mset":  BotAPIEventHandlerWithArgs(manage.SetMuxArgs),
		"mget":  BotAPIEventHandlerWithArgs(manage.GetMuxArgs),
		"v":     BotAPIEventHandlerWithArgs(manage.Version),
		"parse": BotAPIEventHandlerWithArgs(manage.MGlobalPOptions),
		"pause": BotAPIEventHandlerWithArgs(manage.Pause),

		"forward":   BotAPIEventHandlerWithArgs(manage.FCForward),
		"editmedia": BotAPIEventHandlerWithArgs(manage.EditChannelMsg),
		"restart":   BotAPIEventHandlerWithArgs(manage.Restart),
		"nuke":      BotAPIEventHandlerWithArgs(manage.Nuke),
		"rss":       BotAPIEventHandlerWithArgs(manage.RssHandler),

		"bash": BotAPIEventHandlerWithRawArgs(dev.Bash),
		"eval": BotAPIEventHandlerWithRawArgs(dev.Eval),
		"logs": BotAPIEventHandlerWithArgs(dev.GetLogs),
		"sh":   BotAPIEventHandlerWithArgs(dev.Sh),
		"top":  BotAPIEventHandlerWithArgs(dev.Top),

		"download": BotAPIEventHandlerWithArgs(transfer.Download),
		"dl":       BotAPIEventHandlerWithArgs(transfer.Download),
		"rename":   BotAPIEventHandlerWithArgs(transfer.Rename),
		"list":     BotAPIEventHandlerWithArgs(transfer.ListFiles),
		"listv":    BotAPIEventHandlerWithArgs(transfer.ListYtdlFiles),
		"m":        BotAPIEventHandlerWithArgs(transfer.MediaInfo),
		"mux":      BotAPIEventHandlerWithArgs(transfer.Mux),
		"upload":   BotAPIEventHandlerWithArgs(transfer.Upload),
		"ul":       BotAPIEventHandlerWithArgs(transfer.Upload),

		"l":         BotAPIEventHandlerWithArgs(queue.AddLink),
		"ql":        BotAPIEventHandlerWithArgs(queue.AddTorrent),
		"yl":        BotAPIEventHandlerWithArgs(queue.AddWithYtdl),
		"add":       BotAPIEventHandlerWithArgs(queue.AddVideo),
		"queue":     BotAPIEventHandlerWithArgs(queue.ListQueue),
		"batch":     BotAPIEventHandlerWithArgs(queue.EditBatch),
		"clear":     BotAPIEventHandlerWithArgs(queue.ClearQueue),
		"editq":     BotAPIEventHandlerWithArgs(queue.EditQueue),
		"select":    BotAPIEventHandlerWithArgs(queue.Select),
		"cancelall": BotAPIEventHandlerWithArgs(manage.CancelProcesses),

		"anime":  BotAPIEventHandlerWithArgs(info.Anime),
		"airing": BotAPIEventHandlerWithArgs(info.AnimeAiring),
	}
	for cmd, handler := range cmdHandlers {
		pattern := regexp.MustCompile(`^/` + regexp.QuoteMeta(addSuffixcmd(cmd)) + `(?:@\w+)?(?:\s|$)`)
		b.RegisterHandlerRegexp(botapi.HandlerTypeMessageText, pattern, handler)
	}
}

func AddBotAPICBHandlers(b *botapi.Bot) {
	callbackHandlers := map[string]botapi.HandlerFunc{
		"ihelp":     BotAPICallbackEventHandler(handlers.Help),
		"iback":     BotAPICallbackEventHandler(handlers.StartBack),
		"icommands": BotAPICallbackEventHandler(handlers.Commands),

		downloaders.CallbackData:  BotAPICallbackEventHandler(callbacks.CancelDownload),
		downloaders.CallbackDataB: BotAPICallbackEventHandler(callbacks.RevertView),
		downloaders.CallbackDataM: BotAPICallbackEventHandler(callbacks.DLStat),
		downloaders.CallbackDataI: BotAPICallbackEventHandler(callbacks.ViewDlInfo),
		uploaders.CallbackData:    BotAPICallbackEventHandler(callbacks.CancelUpload),

		encoders.CallbackData:             BotAPICallbackEventHandler(callbacks.EncodeInfo),
		encoders.SCallbackData:            BotAPICallbackEventHandler(callbacks.EncodeStats),
		encoders.CancelCallbackData:       BotAPICallbackEventHandler(callbacks.CancelEncode),
		"j" + encoders.CancelCallbackData: BotAPICallbackEventHandler(callbacks.CancelEncodeJobs),

		"cancel_rturn": BotAPICallbackEventHandler(callbacks.CancelRTurn),
		"cancel_mturn": BotAPICallbackEventHandler(callbacks.CancelMTurn),

		batchutils.CallbackData: BotAPICallbackEventHandler(batchutils.PreviewAction),
		queueutils.CallbackData: BotAPICallbackEventHandler(queueutils.PreviewAction),
	}
	for prefix, handler := range callbackHandlers {
		b.RegisterHandler(botapi.HandlerTypeCallbackQueryData, prefix, botapi.MatchTypePrefix, handler)
	}
}

func BotAPIBaseEventHandler(handler func(*tele.Context, *tele.Update, *tele.Message) error) botapi.HandlerFunc {
	return func(ctx context.Context, b *botapi.Bot, update *models.Update) {
		if update == nil || update.Message == nil {
			return
		}
		msg := tele.NewMsgFromBotAPI(update.Message)
		eventCtx := tele.NewBotAPIContext(ctx, b)
		eventUpdate := tele.NewBotAPIUpdate(update)
		if err := handler(eventCtx, eventUpdate, msg); err != nil {
			logger.ErrorW(err.Error())
		}
	}
}

func BotAPICallbackEventHandler(handler func(*tele.Context, *tele.Update, *tele.CallbackQuery) error) botapi.HandlerFunc {
	return func(ctx context.Context, b *botapi.Bot, update *models.Update) {
		if update == nil || update.CallbackQuery == nil {
			return
		}
		eventCtx := tele.NewBotAPIContext(ctx, b)
		eventUpdate := tele.NewBotAPIUpdate(update)
		query := tele.NewQueryFromBotAPI(update.CallbackQuery)
		if err := handler(eventCtx, eventUpdate, query); err != nil {
			logger.ErrorW(err.Error())
		}
	}
}

func BotAPIEventHandlerWithArgs(handler func(*tele.Context, *tele.Update, *tele.Message, string) error) botapi.HandlerFunc {
	return func(ctx context.Context, b *botapi.Bot, update *models.Update) {
		if update == nil || update.Message == nil {
			return
		}
		_, args, _ := strings.Cut(update.Message.Text, " ")
		args = strings.Trim(args, " ")
		msg := tele.NewMsgFromBotAPI(update.Message)
		eventCtx := tele.NewBotAPIContext(ctx, b)
		eventUpdate := tele.NewBotAPIUpdate(update)
		if err := handler(eventCtx, eventUpdate, msg, args); err != nil {
			logger.ErrorW(err.Error())
		}
	}
}

func BotAPIEventHandlerWithRawArgs(handler func(*tele.Context, *tele.Update, *tele.Message, string) error) botapi.HandlerFunc {
	return BotAPIEventHandlerWithArgs(handler)
}
