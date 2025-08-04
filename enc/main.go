package main

import (
	"log"
	"strings"

	"enc/config"
	"enc/internal/logger"
)

func main() {
	if err := logger.Init("logs.txt"); err != nil {
		log.Fatalf("logger init error: %v", err)
	}
	defer logger.Logger.Sync()
	if err := config.LoadConfig(); err != nil {
		logger.CriticalW("config error: %v", err)
	}
	logger.Logger.Info(strings.Repeat("=", 30))

	// … rest of your app …
}
