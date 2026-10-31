package anitomy

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"

	"enc/internal/logger"

	"go.uber.org/zap"
)

func Parse(name string) *Elements {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx, "anitomy",
		"--format=json",
		name,
	)
	var elements Elements
	out, err := cmd.Output()
	if err != nil {
		logger.WarnW("Error while running anitomy", zap.Error(err))
		return &elements
	}
	if err := json.Unmarshal([]byte(out), &elements); err != nil {
		logger.WarnW("Error while Unmarshalling json from anitomy", zap.Error(err))
	}
	return &elements
}
