package startup

import (
	"context"

	"github.com/gotd/td/tg"

	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
)

func AddAllCommands() {
	commands := bot.ParseCommands(bot.GetBotCommands())
	rawClient := telegram.Client.Client.API()
	request := &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: "en",
		Commands: commands,
	}
	resp, err := rawClient.BotsSetBotCommands(context.Background(), request)
	if resp {
		logger.InfoW("Available Commands Set successfully")
	} else if err != nil {
		logger.ErrorW(err.Error())
	}
}
