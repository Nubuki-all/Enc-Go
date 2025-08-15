package startup


import (
	"github.com/gotd/td/tg"

	"enc/internal/clients/telegram"
	"enc/internal/logger"
	"enc/internal/bot"
)


func AddAllCommands() {
    commands := bot.ParseCommands(bot.GetBotCommands())
    rawClient := telegram.Client.Client.API()
    resp := rawClient.BotsSetBotCommands(tg.BotCommandScopeDefault, "en", commands)
    logger.Info("BotsSetBotCommands Returned:", resp)
}


