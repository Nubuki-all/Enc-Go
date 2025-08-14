package main

import (
	"context"
	"log"
	"strings"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/clients/telegraph"
	"enc/internal/logger"
	"enc/internal/startup"
)

func main() {
	if err := logger.Init(config.Conf.LogFileName, config.Conf.JsonLog); err != nil {
		log.Fatalf("logger init error: %v", err)
	}
	defer logger.Logger.Sync()
	logger.Logger.Info(strings.Repeat("=", 30))
	if config.Rconf.Error != nil {
		logger.Criticalf("Failed to load config: %v", config.Rconf.Error)
	}
	bot.Version = Version
	startup.Initialize()
	telegraph.Init()
	telegram.InitClient()
	startup.Finalize(context.Background())
	telegram.Client.Idle()

	// … rest of your app …
}
