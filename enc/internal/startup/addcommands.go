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
	logger.Info("BotsSetBotCommands Returned:", resp, err)
}
