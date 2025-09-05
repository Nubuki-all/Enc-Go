package main

import (
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
	defer func() { _ = logger.Logger.Sync() }()
	logger.Info("Starting up" + strings.Repeat(".", 10))
	if config.Rconf.Error != nil {
		logger.Criticalf("Failed to load config: %v", config.Rconf.Error)
	}
	bot.Version = Version
	startup.Initialize()
	telegraph.Init()
	telegram.InitClient()
	startup.Finalize(telegram.Context)
	defer bot.Aria2Client.Close() //nolint:errcheck
	err := telegram.Client.Idle()
	if err != nil {
		logger.Error(err)
	}

	// … rest of your app …
}
