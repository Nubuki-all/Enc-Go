package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"strings"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/clients/telegram"
	"enc/internal/clients/telegraph"
	"enc/internal/logger"
	"enc/internal/startup"

	"go.uber.org/zap"
)

func main() {
	if err := logger.Init(config.Conf.LogFileName, config.Conf.JSONLog); err != nil {
		log.Fatalf("logger init error: %v", err)
	}
	defer func() { _ = logger.Logger.Sync() }()

	if config.Conf.EnablePPROF {
		go func() {
			listenAddr := "localhost:6060"
			logger.InfoW("Starting pprof server on", zap.String("Addr", listenAddr))
			if err := http.ListenAndServe(listenAddr, nil); err != nil {
				logger.ErrorW("pprof server error", zap.Error(err))
			}
		}()
	}

	logger.Info(strings.Repeat(".", 20))
	logger.InfoW("Starting bot...", zap.String("Version", Version))
	if config.Rconf.Error != nil {
		logger.Criticalf("Failed to load config: %v", config.Rconf.Error)
	}

	bot.Version = Version
	startup.Initialize()
	extract()
	telegraph.Init()
	telegram.InitClient()
	startup.Finalize(telegram.Context)
	defer bot.Aria2Client.Close() //nolint:errcheck
	telegram.Idle()
}
