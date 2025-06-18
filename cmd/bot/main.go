// ====================================
// File: cmd/bot/main.go
// ====================================
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/rovshanmuradov/solana-bot/internal/bot"
	"github.com/rovshanmuradov/solana-bot/internal/logger"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

func main() {
	// Флаг конфигурации
	configPath := flag.String("config", "configs/config.json", "Path to config file")
	flag.Parse()

	// Контекст с обработкой SIGINT / SIGTERM
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Загрузка конфига
	cfg, err := task.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Логгер
	appLogger, err := logger.CreatePrettyLogger(cfg.DebugLogging)
	if err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer func() {
		_ = appLogger.Sync()
	}()

	// Create unified bot service
	botService, err := bot.NewBotService(rootCtx, &bot.BotServiceConfig{
		Config: cfg,
		Logger: appLogger,
	})
	if err != nil {
		log.Fatalf("💥 Failed to create bot service: %v", err)
	}

	// Start the bot service
	if err := botService.Start(); err != nil {
		log.Fatalf("💥 Failed to start bot service: %v", err)
	}

	appLogger.Info("🚀 Bot service started, using old runner for task execution")

	// For now, still use the old runner for backward compatibility
	// TODO: In future versions, migrate task execution to use BotService.HandleCommand()
	runner := bot.NewRunner(cfg, appLogger)
	if err := runner.Run(rootCtx); err != nil && rootCtx.Err() == nil {
		// Shutdown bot service on error
		if shutdownErr := botService.Shutdown(rootCtx); shutdownErr != nil {
			appLogger.Error("Failed to shutdown bot service", zap.Error(shutdownErr))
		}
		log.Fatalf("💥 Application failed to start: %v", err)
	}

	// Graceful shutdown
	if err := botService.Shutdown(rootCtx); err != nil {
		appLogger.Error("Failed to shutdown bot service", zap.Error(err))
	}
}
