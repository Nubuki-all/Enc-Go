package main

import (
	"context"
	"embed"
	"filepath"
	"os"
	"time"

	"enc/internal/logger"
	"enc/internal/startup"
	"enc/utils"
)

//go:embed resources
var resources embed.FS

func extract() {
	if err := utils.ExtractEmbedDotPrefix(resources, "resources", ""); err != nil {
		logger.Critical("extract failed: %v", err)
	} else {
		p, err := filepath.Abs(".bin")
		if err != nil {
			logger.Critical("updating path failed: %v", err)
		}
		sep := string(os.PathListSeparator) // ":" on Unix, ";" on Windows
		old := os.Getenv("PATH")
		newPath := p
		if old != "" {
			newPath = p + sep + old
		}
		if err := os.Setenv("PATH", newPath); err != nil {
			logger.Critical("updating path failed: %v", err)
		}
		ctx, _ := context.WithTimeout(context.Background(), 180*time.Second)
		if _, err := startup.InstallAria2(ctx, p); err != nil {
			logger.Error(err)
		}
		if _, err := startup.InstallQbittorrentNox(ctx, p); err != nil {
			logger.Error(err)
		}
	}
}
