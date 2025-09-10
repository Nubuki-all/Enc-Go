package main

import (
	"embed"
	"os"

	"enc/internal/logger"
	"enc/utils"
)

//go:embed resources
var resources embed.FS

func extract() {
	if err := utils.ExtractEmbedDotPrefix(resources, "resources", ""); err != nil {
		logger.Critical("extract failed: %v", err)
	} else {
		p := ".bin"
		sep := string(os.PathListSeparator) // ":" on Unix, ";" on Windows
		old := os.Getenv("PATH")
		newPath := p
		if old != "" {
			newPath = p + sep + old
		}
		if err := os.Setenv("PATH", newPath); err != nil {
			logger.Critical("updating path failed: %v", err)
		}
	}
}
