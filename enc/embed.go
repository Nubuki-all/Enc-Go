package main

import (
	"embed"

	"enc/internal/logger"
	"enc/utils"
)

//go:embed resources
var resources embed.FS

func extract() {
	if err := utils.ExtractEmbedDotPrefix(resources, "resources", ""); err != nil {
		logger.Critical("extract failed: %v", err)
	}
}
