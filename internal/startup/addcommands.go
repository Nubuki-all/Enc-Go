package startup

import (
	//"context"

	"github.com/gotd/td/tg"

	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
)

func AddAllCommands() {
	commands := bot.ParseCommands(bot.GetBotCommands())
	ctx := telegram.Context
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
