package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

func setupLog(verbose bool) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig = newEncoderConfig()
	if verbose {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	cfg.Encoding = "json"
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	zapLogger, _ := cfg.Build()
	logger = zapLogger.Sugar()
	return zapLogger
}

func newEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()
	cfg.TimeKey = "timestamp"
	cfg.LevelKey = "level"
	cfg.MessageKey = "message"
	cfg.EncodeTime = zapcore.RFC3339TimeEncoder

	return cfg
}
