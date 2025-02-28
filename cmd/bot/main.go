// ====================================
// File: cmd/bot/main.go (refactored)
// ====================================
package main

import (
	"context"
	"os"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/bot"
)

func main() {
	// Setup context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	logger.Info("Starting sniper bot")

	// Create and initialize bot runner
	runner := bot.NewRunner(logger)
	if err := runner.Initialize("configs/config.json"); err != nil {
		logger.Fatal("Failed to initialize bot", zap.Error(err))
		os.Exit(1)
	}

	// Run the bot
	if err := runner.Run(ctx); err != nil {
		logger.Fatal("Bot execution error", zap.Error(err))
		os.Exit(1)
	}

	// Wait for shutdown (alternatively, this could be part of Run())
	runner.WaitForShutdown()
}
