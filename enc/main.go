package main

import (
	"log"
	"strings"

	"enc/config"
	"enc/internal/logger"
)

func main() {
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("config error: %v", err)
	}

	// 2. init your logger
	if err := logger.Init("logs.txt"); err != nil {
		log.Fatalf("logger init error: %v", err)
	}
	defer logger.Logger.Sync()
	logger.Logger.Info(strings.Repeat("=", 30))

	// … rest of your app …
}
