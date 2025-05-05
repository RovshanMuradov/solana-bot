// ====================================
// File: cmd/bot/main.go ( исправленный)
// ====================================
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore" // <<< ДОБАВЛЕНО для zapcore.DebugLevel/InfoLevel

	"github.com/rovshanmuradov/solana-bot/internal/bot"
	"github.com/rovshanmuradov/solana-bot/internal/task" // <<< ДОБАВЛЕНО для загрузки конфига
)

func main() {
	// Setup context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// <<< ИЗМЕНЕНО: Сначала загружаем конфиг >>>
	configPath := "configs/config.json"
	cfg, err := task.LoadConfig(configPath)
	if err != nil {
		// На этом этапе логгер еще не создан, выводим в stderr
		fmt.Fprintf(os.Stderr, "Failed to load configuration from %s: %v\n", configPath, err)
		os.Exit(1)
	}

	// Initialize logger <<< ИЗМЕНЕНО: Используем cfg для установки уровня >>>
	logConfig := zap.NewProductionConfig()

	// <<< ИЗМЕНЕНО: Устанавливаем уровень на основе конфига >>>
	logLevel := zapcore.InfoLevel // Уровень по умолчанию INFO
	if cfg.DebugLogging {
		logLevel = zapcore.DebugLevel // Меняем на DEBUG, если в конфиге true
	}
	logConfig.Level = zap.NewAtomicLevelAt(logLevel)

	// <<< ДОБАВЛЕНО: Можно также настроить формат времени и кодировщик для лучшей читаемости, если нужно >>>
	logConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // Более читаемый формат времени
	// logConfig.Encoding = "console" // Если предпочитаешь не-JSON формат

	logger, err := logConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build logger: %v\n", err)
		os.Exit(1)
	}
	// =====================================================

	defer func() {
		// Правильная обработка ошибки Sync
		if syncErr := logger.Sync(); syncErr != nil {
			// Игнорируем стандартные ошибки при синхронизации логгера
			errMsg := syncErr.Error()
			if !strings.Contains(errMsg, "invalid argument") &&
				!strings.Contains(errMsg, "inappropriate ioctl for device") &&
				!strings.Contains(errMsg, "bad file descriptor") { // Добавлено для некоторых систем
				fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", syncErr)
			}
		}
	}()

	logger.Info("Starting sniper bot", zap.Bool("debugLogging", cfg.DebugLogging), zap.String("logLevel", logLevel.String())) // Логируем активный уровень

	// Create and initialize bot runner
	runner := bot.NewRunner(logger)
	// <<< ИЗМЕНЕНО: Используем уже загруженный cfg и передаем путь только для единообразия, если Initialize его использует >>>
	// Если Initialize использует cfg напрямую, можно убрать path отсюда
	if err := runner.Initialize(configPath); err != nil { // Initialize теперь может использовать cfg из runner'а
		logger.Fatal("Failed to initialize bot", zap.Error(err))
	}

	// Run the bot
	if err := runner.Run(ctx); err != nil {
		logger.Fatal("Bot execution error", zap.Error(err))
	}

	// Wait for shutdown (alternatively, this could be part of Run())
	logger.Info("Bot finished execution, waiting for shutdown signal...") // Добавлено для ясности
	runner.WaitForShutdown()
	logger.Info("Shutdown complete.")
}
