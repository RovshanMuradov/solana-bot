// internal/utils/logger.go
package utils

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLogger(debug bool, logFile string) (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	if debug {
		config = zap.NewDevelopmentConfig()
	}

	// Настройка вывода в консоль и файл
	consoleEncoder := zapcore.NewConsoleEncoder(config.EncoderConfig)
	fileEncoder := zapcore.NewJSONEncoder(config.EncoderConfig)

	// Открытие файла для логирования
	logFileHandle, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), config.Level),
		zapcore.NewCore(fileEncoder, zapcore.AddSync(logFileHandle), config.Level),
	)

	logger := zap.New(core)
	return logger, nil
}
