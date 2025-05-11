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
	logger, err := newLogger(cfg.DebugLogging)
	if err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Runner
	runner := bot.NewRunner(cfg, logger)
	if err := runner.Run(rootCtx); err != nil && rootCtx.Err() == nil {
		logger.Fatal("Runner failed", zap.Error(err))
	}
}
func newLogger(debug bool) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if debug {
		cfg = zap.NewDevelopmentConfig()
	}
	cfg.OutputPaths = []string{"stdout"}
	return cfg.Build()
}
