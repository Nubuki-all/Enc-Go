// You can edit this code!
// Click here and start typing.
package main


// Init sets up the global zap logger (and a sugared alias),
// writing to stdout + a rotating file

import (
    "os"

    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    "gopkg.in/natefinch/lumberjack.v2"
)

var (
    Logger *zap.Logger
    Sugar  *zap.SugaredLogger
)

func Init(logFile string, useJSON bool) error {
    // Rotating file output
    lj := &lumberjack.Logger{
        Filename:   logFile,
        MaxSize:    100,
        MaxBackups: 7,
        MaxAge:     28,
        Compress:   true,
    }

    // Shared encoder settings
    encCfg := zapcore.EncoderConfig{
        TimeKey:       "ts",
        LevelKey:      "level",
        CallerKey:     "caller",
        MessageKey:    "msg",
        StacktraceKey: "stacktrace",
        LineEnding:    zapcore.DefaultLineEnding,

        // colorized levels in console
        EncodeLevel: zapcore.CapitalColorLevelEncoder,
        // custom timestamp format
        EncodeTime: zapcore.TimeEncoderOfLayout("02-Jan-06 15:04:05"),
        // default file:line for caller
        EncodeCaller: zapcore.ShortCallerEncoder,
    }

    // Console (color)
    consoleEnc := zapcore.NewConsoleEncoder(encCfg)
    consoleWS  := zapcore.AddSync(os.Stdout)

    // File (no color)
    fileCfg := encCfg
    fileCfg.EncodeLevel = zapcore.CapitalLevelEncoder
    var fileEnc zapcore.Encoder
    if useJSON {
        fileEnc = zapcore.NewJSONEncoder(fileCfg)
    } else {
        fileEnc = zapcore.NewConsoleEncoder(fileCfg)
    }
    fileWS := zapcore.AddSync(lj)

    // Cores
    consoleCore := zapcore.NewCore(consoleEnc, consoleWS, zapcore.InfoLevel)
    fileCore    := zapcore.NewCore(fileEnc,    fileWS,    zapcore.InfoLevel)

    // Combine them
    core := zapcore.NewTee(consoleCore, fileCore)

    // Build the logger
    Logger = zap.New(core,
        zap.AddCaller(),                // include caller
        zap.AddStacktrace(zapcore.ErrorLevel),
    )
    Sugar = Logger.Sugar()
    zap.ReplaceGlobals(Logger)
    return nil
}

// Info logs a message at InfoLevel. Arguments are handled in fmt.Sprint.
func Info(args ...interface{}) {
    Sugar.Info(args...)
}

// Infof logs a formatted message at InfoLevel.
func Infof(template string, args ...interface{}) {
    Sugar.Infof(template, args...)
}

// Debug logs a message at DebugLevel.
func Debug(args ...interface{}) {
    Sugar.Debug(args...)
}

// Debugf logs a formatted message at DebugLevel.
func Debugf(template string, args ...interface{}) {
    Sugar.Debugf(template, args...)
}

// Warn logs a message at WarnLevel.
func Warn(args ...interface{}) {
    Sugar.Warn(args...)
}

// Warnf logs a formatted message at WarnLevel.
func Warnf(template string, args ...interface{}) {
    Sugar.Warnf(template, args...)
}

// Error logs a message at ErrorLevel.
func Error(args ...interface{}) {
    Sugar.Error(args...)
}

// Errorf logs a formatted message at ErrorLevel.
func Errorf(template string, args ...interface{}) {
    Sugar.Errorf(template, args...)
}

// Critical logs a message at FatalLevel and then calls os.Exit(1).
func Critical(args ...interface{}) {
    Sugar.Fatal(args...)
}

// Criticalf logs a formatted message at FatalLevel and then calls os.Exit(1).
func Criticalf(template string, args ...interface{}) {
    Sugar.Fatalf(template, args...)
}

// InfoW logs a message with structured context.
// Usage: logger.InfoW("Queue loaded from MongoDB", zap.Int("count", len(result.Queue)))
func InfoW(msg string, fields ...zap.Field) {
    Logger.Info(msg, fields...)
}

// Likewise for WarnW, ErrorW, DebugW, CriticalW:
func WarnW(msg string, fields ...zap.Field) {
    Logger.Warn(msg, fields...)
}

func ErrorW(msg string, fields ...zap.Field) {
    Logger.Error(msg, fields...)
}

func DebugW(msg string, fields ...zap.Field) {
    Logger.Debug(msg, fields...)
}

// CriticalW uses Fatal (which calls os.Exit after logging).
func CriticalW(msg string, fields ...zap.Field) {
    Logger.Fatal(msg, fields...)
}



