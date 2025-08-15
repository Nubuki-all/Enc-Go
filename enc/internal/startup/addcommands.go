package startup

import (
	"github.com/gotd/td/tg"

	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/logger"
)

func AddAllCommands() {
	commands := bot.ParseCommands(bot.GetBotCommands())
	rawClient := telegram.Client.Client.API()
	resp := rawClient.BotsSetBotCommands(tg.BotCommandScopeDefault, "en", commands)
	logger.Info("BotsSetBotCommands Returned:", resp)
}
