// cmd/test_commands/main.go
package main

import (
	"log"

	"github.com/rovshanmuradov/solana-bot/internal/bot"
	"go.uber.org/zap"
)

func main() {
	// Создаем logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting Command/Event system test")

	// Запускаем демонстрацию
	if err := bot.DemoCommandEventSystem(logger); err != nil {
		logger.Fatal("Demo failed", zap.Error(err))
	}

	logger.Info("Command/Event system test completed successfully")
}
