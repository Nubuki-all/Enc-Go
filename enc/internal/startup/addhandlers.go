package startup

import (
	"strings"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/celestix/gotgproto/dispatcher"
	onevent "github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"enc/config"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
	batchutils "enc/utils/batch"
	queueutils "enc/utils/queue"
	"enc/utils/tele"
	"enc/workers/handlers"
	"enc/workers/handlers/callbacks"
	"enc/workers/handlers/dev"
	"enc/workers/handlers/info"
	"enc/workers/handlers/manage"
	"enc/workers/handlers/queue"
	"enc/workers/handlers/transfer"
)

func AddAllHandlers() {
	d := telegram.Client.Dispatcher
	AddCmdHandlers(d); AddCBHandlers(d)
	telegram.HandleBotAPIUpdate = HandleBotAPIUpdate
}

func HandleBotAPIUpdate(update tgbotapi.Update) {
	if update.Message != nil { handleBotAPIMessage(update.Message) } else if update.CallbackQuery != nil { handleBotAPICallbackQuery(update.CallbackQuery) }
}

func handleBotAPIMessage(m *tgbotapi.Message) {
	msg := tele.NewMsgFromBotAPI(m)
	if m.IsCommand() {
		cmd := strings.TrimSuffix(m.Command(), config.Conf.CmdSuffix)
		if handler, ok := botAPICmdHandlers[cmd]; ok { _ = handler(msg) }
	} else if m.Document != nil || m.Video != nil || m.Audio != nil || m.Photo != nil { _ = queue.AutoAddDocument(msg) }
}

func handleBotAPICallbackQuery(q *tgbotapi.CallbackQuery) {
	cb := tele.NewQueryFromBotAPI(q)
	for prefix, handler := range botAPICBHandlers { if strings.HasPrefix(q.Data, prefix) { _ = handler(cb); return } }
}

var botAPICmdHandlers = make(map[string]func(*tele.Message) error)
var botAPICBHandlers = make(map[string]func(*tele.CallbackQuery) error)

func AddCmdHandlers(dispatch dispatcher.Dispatcher) {
	cmdHandlers := map[string]func(*tele.Message) error{
		"start": handlers.Start, "help": handlers.Start, "ping": handlers.Up, "status": handlers.Status,
		"unpermit": EventHandlerWithArgs(manage.TempUnauth), "permit": EventHandlerWithArgs(manage.TempAuth),
		"bash": EventHandlerWithRawArgs(dev.Bash), "eval": EventHandlerWithRawArgs(dev.Eval),
		"download": EventHandlerWithArgs(transfer.Download), "dl": EventHandlerWithArgs(transfer.Download),
		"add": EventHandlerWithArgs(queue.AddVideo),
	}
	for cmd, handler := range cmdHandlers {
		botAPICmdHandlers[cmd] = handler
		dispatch.AddHandler(onevent.NewCommand(cmd+config.Conf.CmdSuffix, BaseEventHandler(handler)))
	}
	dispatch.AddHandler(onevent.NewMessage(filters.Message.Media, BaseEventHandler(queue.AutoAddDocument)))
}

func AddCBHandlers(dispatch dispatcher.Dispatcher) {
	callbackHandlers := map[string]func(*tele.CallbackQuery) error{
		"ihelp": handlers.Help, "iback": handlers.StartBack, "icommands": handlers.Commands,
		batchutils.CallbackData: batchutils.PreviewAction, queueutils.CallbackData: queueutils.PreviewAction,
	}
	for prefix, handler := range callbackHandlers {
		botAPICBHandlers[prefix] = handler
		dispatch.AddHandler(onevent.NewCallbackQuery(filters.CallbackQuery.Prefix(prefix), CallbackEventHandler(handler)))
	}
}

func BaseEventHandler(handler func(*tele.Message) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		msg := tele.NewMsgFromGotg(update.EffectiveMessage, ctx, update.EffectiveChat().GetID(), tele.GetSenderIDFromMessage(update.EffectiveMessage))
		return handler(msg)
	}
}

func CallbackEventHandler(handler func(*tele.CallbackQuery) error) func(*ext.Context, *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		query := tele.NewQueryFromGotg(update.CallbackQuery, ctx, update.EffectiveChat().GetID(), update.EffectiveUser().ID)
		return handler(query)
	}
}

func EventHandlerWithArgs(handler func(*tele.Message, string) error) func(*tele.Message) error {
	return func(msg *tele.Message) error {
		text, _ := msg.RawText(); _, args, _ := strings.Cut(text, " "); return handler(msg, strings.TrimSpace(args))
	}
}

func EventHandlerWithRawArgs(handler func(*tele.Message, string) error) func(*tele.Message) error {
	return func(msg *tele.Message) error {
		text, _ := msg.RawText(); _, args, _ := strings.Cut(text, " "); return handler(msg, strings.TrimSpace(args))
	}
}
