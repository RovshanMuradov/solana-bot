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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/bot"
	"github.com/rovshanmuradov/solana-bot/internal/logger"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/state"
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

	// Initialize LogBuffer before logger creation
	logBufferPath := "logs/app.log.buffer"
	logBuffer, err := logger.NewLogBuffer(1000, logBufferPath, nil) // Will set logger later
	if err != nil {
		log.Fatalf("Failed to create log buffer: %v", err)
	}

	// Логгер with LogBuffer integration - use TUI-compatible logger
	// TUI mode detected: only write to buffer, no console output to avoid breaking UI
	appLogger, err := logger.CreateTUILoggerWithBuffer(cfg.DebugLogging, logBuffer)
	if err != nil {
		// Fallback to regular logger if TUI logger fails
		appLogger, err = logger.CreatePrettyLoggerWithBuffer(cfg.DebugLogging, logBuffer)
		if err != nil {
			log.Fatalf("Failed to init logger: %v", err)
		}
	}
	logBuffer.SetLogger(appLogger) // Set logger after creation

	// Initialize shutdown handler FIRST
	shutdownHandler := bot.GetShutdownHandler()
	shutdownHandler.SetLogger(appLogger)

	// Initialize global communication (Phase 2)
	msgChan := make(chan tea.Msg, 1000) // Buffer for UI messages
	ui.InitBus(msgChan, appLogger)
	state.InitCache(appLogger)

	// Register Global Bus and Cache with shutdown handler
	shutdownHandler.RegisterService("globalBus", &globalBusCloser{ui.GlobalBus})
	shutdownHandler.RegisterService("globalCache", &globalCacheCloser{state.GlobalCache})

	// Register logger and buffer for graceful shutdown
	shutdownHandler.RegisterService("logBuffer", logBuffer)
	shutdownHandler.RegisterService("logger", &loggerCloser{appLogger})

	// Create unified bot service
	botService, err := bot.NewBotService(rootCtx, &bot.BotServiceConfig{
		Config:           cfg,
		Logger:           appLogger,
		UIMessageChannel: msgChan, // Phase 2: Provide message channel for throttling
	})
	if err != nil {
		log.Fatalf("💥 Failed to create bot service: %v", err)
	}

	// Register bot service for shutdown
	shutdownHandler.RegisterService("botService", botService)

	// Start the bot service
	if err := botService.Start(); err != nil {
		log.Fatalf("💥 Failed to start bot service: %v", err)
	}

	appLogger.Info("🚀 Bot service started with enhanced safety features")

	// Create runner
	runner := bot.NewRunner(cfg, appLogger)

	// Run in goroutine to allow shutdown handler to work
	runnerDone := make(chan error, 1)
	go func() {
		runnerDone <- runner.Run(rootCtx)
	}()

	// Wait for either runner completion or shutdown signal
	select {
	case err := <-runnerDone:
		if err != nil && rootCtx.Err() == nil {
			appLogger.Error("Runner failed", zap.Error(err))
			// Trigger graceful shutdown
			shutdownHandler.InitiateShutdown()
		}
	case <-shutdownHandler.ShutdownInitiated():
		appLogger.Info("Shutdown signal received")
	}

	// GRACEFUL SHUTDOWN - This will shutdown all registered services in LIFO order
	shutdownHandler.WaitForShutdown()
}

// Helper wrapper for logger
type loggerCloser struct{ *zap.Logger }

func (l *loggerCloser) Close() error {
	return l.Sync()
}

// Helper wrapper for global bus
type globalBusCloser struct{ *ui.NonBlockingBus }

func (g *globalBusCloser) Close() error {
	// Close the global bus (no error returned by Close())
	g.NonBlockingBus.Close()
	return nil
}

// Helper wrapper for global cache
type globalCacheCloser struct{ *state.UICache }

func (g *globalCacheCloser) Close() error {
	// Clear cache on shutdown (no persistent state to save)
	g.UICache.Clear()
	return nil
}
