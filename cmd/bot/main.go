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

	// Runner
	runner := bot.NewRunner(cfg, appLogger)
	if err := runner.Run(rootCtx); err != nil && rootCtx.Err() == nil {
		log.Fatalf("💥 Application failed to start: %v", err)
	}
}
