package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
)

// Init sets up the global zap logger (and a sugared alias),
// writing to stdout + a rotating file.
func Init(logFile string) error {
	lj := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    100,
		MaxBackups: 7,
		MaxAge:     28,
		Compress:   true,
	}

	encCfg := zapcore.EncoderConfig{
		TimeKey:       "ts",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "caller",
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.CapitalColorLevelEncoder,
		EncodeTime:    zapcore.TimeEncoderOfLayout(time.RFC3339),
		EncodeCaller:  zapcore.ShortCallerEncoder,
	}

	consoleEnc := zapcore.NewConsoleEncoder(encCfg)
	ws := zapcore.NewMultiWriteSyncer(
		zapcore.AddSync(os.Stdout),
		zapcore.AddSync(lj),
	)
	core := zapcore.NewCore(consoleEnc, ws, zapcore.InfoLevel)

	Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()

	// replace the package-level globals in zap itself
	zap.ReplaceGlobals(Logger)

	return nil
}
