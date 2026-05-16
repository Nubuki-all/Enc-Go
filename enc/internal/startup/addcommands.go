package startup

import (
	"context"

	botapi "enc/internal/clients/telegram/botapi"
	"enc/internal/clients/telegram/botapi/models"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
)

func AddAllCommands() {
	if config.Conf.TgDispatcher.UsesBotAPI() {
		addBotAPICommands()
		return
	}
	if config.Conf.TgDispatcher.UsesGotg() {
		addGotgCommands()
	}
}

func addGotgCommands() {
	commands := bot.ParseCommands(bot.GetBotCommands())
	ctx := telegram.Context
	if ctx == nil {
		logger.WarnW("gotgproto command registration skipped: client is not initialized")
		return
	}
	request := &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: "en",
		Commands: commands,
	}
	resp, err := ctx.Raw.BotsSetBotCommands(ctx, request)
	if resp {
		logger.InfoW("Available Commands Set successfully")
	} else if err != nil {
		logger.ErrorW(err.Error())
	}
}

func addBotAPICommands() {
	if telegram.BotAPI == nil {
		logger.WarnW("bot api command registration skipped: client is not initialized")
		return
	}
	gotgCommands := bot.ParseCommands(bot.GetBotCommands())
	commands := make([]models.BotCommand, 0, len(gotgCommands))
	for _, cmd := range gotgCommands {
		commands = append(commands, models.BotCommand{Command: cmd.Command, Description: cmd.Description})
	}
	_, err := telegram.BotAPI.SetMyCommands(context.Background(), &botapi.SetMyCommandsParams{Commands: commands})
	if err != nil {
		logger.ErrorW(err.Error())
		return
	}
	logger.InfoW("Available Commands Set successfully")
}
