// ====================================
// File: cmd/bot/main.go (refactored)
// ====================================
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/bot"
)

func main() {
	// Setup context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer func() {
		// Правильная обработка ошибки Sync
		if err := logger.Sync(); err != nil {
			// Игнорируем ошибку "sync /dev/stdout: invalid argument"
			if !strings.Contains(err.Error(), "invalid argument") {
				fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", err)
			}
		}
	}()
	logger.Info("Starting sniper bot")

	// Create and initialize bot runner
	runner := bot.NewRunner(logger)
	if err := runner.Initialize("configs/config.json"); err != nil {
		logger.Fatal("Failed to initialize bot", zap.Error(err))
		// Не нужен os.Exit, так как logger.Fatal уже вызывает os.Exit(1)
	}

	// Run the bot
	if err := runner.Run(ctx); err != nil {
		logger.Fatal("Bot execution error", zap.Error(err))
		// Не нужен os.Exit, так как logger.Fatal уже вызывает os.Exit(1)
	}

	// Wait for shutdown (alternatively, this could be part of Run())
	runner.WaitForShutdown()
}
