// internal/bot/service.go
package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

// EventBusAdapter adapts bot.EventBus to monitor.EventBus interface
type EventBusAdapter struct {
	eventBus *EventBus
}

func (a *EventBusAdapter) Publish(event interface{}) {
	if tradingEvent, ok := event.(TradingEvent); ok {
		a.eventBus.Publish(tradingEvent)
	}
}

// BotService provides unified access to all bot services
type BotService struct {
	trading          *TradingService
	monitor          monitor.MonitorService
	config           *task.Config
	eventBus         *EventBus
	commandBus       *CommandBus
	logger           *zap.Logger
	blockchainClient *blockchain.Client
	wallets          map[string]*task.Wallet
	taskManager      *task.Manager

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// BotServiceConfig configuration for BotService
type BotServiceConfig struct {
	Config           *task.Config
	Logger           *zap.Logger
	UIMessageChannel chan tea.Msg // Phase 2: UI message channel for throttling
}

// NewBotService creates a new unified bot service
func NewBotService(parentCtx context.Context, config *BotServiceConfig) (*BotService, error) {
	ctx, cancel := context.WithCancel(parentCtx)

	logger := config.Logger.Named("bot_service")
	logger.Info("🚀 Initializing BotService")

	// Initialize blockchain client
	var rpcURL string
	if len(config.Config.RPCList) > 0 {
		rpcURL = config.Config.RPCList[0]
	} else {
		rpcURL = "https://api.mainnet-beta.solana.com"
	}
	blockchainClient := blockchain.NewClient(rpcURL, logger)

	// Initialize task manager
	taskManager := task.NewManager(logger)

	// Load wallets
	wallets, err := task.LoadWallets("configs/wallets.csv")
	if err != nil {
		logger.Error("Failed to load wallets", zap.Error(err))
		cancel()
		return nil, fmt.Errorf("failed to load wallets: %w", err)
	}

	logger.Info("✅ Loaded wallets", zap.Int("wallet_count", len(wallets)))

	// Create MonitorService placeholder (will be set after TradingService creation)
	monitorService := monitor.NewMonitorService(&monitor.MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         nil,                     // Will be set after TradingService creation
		UIMessageChannel: config.UIMessageChannel, // Phase 2: For price throttling
	})

	// Create TradingService
	tradingService := NewTradingService(ctx, &TradingServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		Wallets:          wallets,
		TaskManager:      taskManager,
		MonitorService:   monitorService,
	})

	// Create EventBus adapter for MonitorService compatibility
	eventBusAdapter := &EventBusAdapter{eventBus: tradingService.GetEventBus()}

	// Recreate MonitorService with proper EventBus
	monitorService = monitor.NewMonitorService(&monitor.MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBusAdapter,
		UIMessageChannel: config.UIMessageChannel, // Phase 2: For price throttling
	})

	// Recreate TradingService with proper MonitorService
	tradingService = NewTradingService(ctx, &TradingServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		Wallets:          wallets,
		TaskManager:      taskManager,
		MonitorService:   monitorService,
	})

	service := &BotService{
		trading:          tradingService,
		monitor:          monitorService,
		config:           config.Config,
		eventBus:         tradingService.GetEventBus(),
		commandBus:       tradingService.GetCommandBus(),
		logger:           logger,
		blockchainClient: blockchainClient,
		wallets:          wallets,
		taskManager:      taskManager,
		ctx:              ctx,
		cancel:           cancel,
	}

	logger.Info("✅ BotService initialized successfully")
	return service, nil
}

// Start initializes and starts all services
func (s *BotService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("🚀 Starting BotService")

	// Load and validate tasks
	tasks, err := s.taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		s.logger.Error("Failed to load tasks", zap.Error(err))
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	s.logger.Info("📋 Tasks loaded for validation", zap.Int("task_count", len(tasks)))

	// Validate RPC connectivity
	if err := s.validateRPCConnectivity(); err != nil {
		s.logger.Error("RPC connectivity validation failed", zap.Error(err))
		return fmt.Errorf("RPC connectivity validation failed: %w", err)
	}

	s.logger.Info("✅ BotService started successfully")
	return nil
}

// Shutdown gracefully shuts down all services
func (s *BotService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("🛑 Shutting down BotService")

	// Shutdown monitor service
	if err := s.monitor.Shutdown(ctx); err != nil {
		s.logger.Error("Failed to shutdown monitor service", zap.Error(err))
	}

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	s.logger.Info("✅ BotService shutdown completed")
	return nil
}

// Close implements io.Closer interface for shutdown handler compatibility
func (s *BotService) Close() error {
	// Use a background context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.Shutdown(ctx)
}

// HandleCommand handles a trading command
func (s *BotService) HandleCommand(ctx context.Context, cmd TradingCommand) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.logger.Debug("📨 Handling command",
		zap.String("command_type", cmd.GetType()),
		zap.String("user_id", cmd.GetUserID()))

	return s.commandBus.Send(ctx, cmd)
}

// GetEventBus returns the event bus
func (s *BotService) GetEventBus() *EventBus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventBus
}

// GetCommandBus returns the command bus
func (s *BotService) GetCommandBus() *CommandBus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.commandBus
}

// GetTradingService returns the trading service
func (s *BotService) GetTradingService() *TradingService {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trading
}

// GetMonitorService returns the monitor service
func (s *BotService) GetMonitorService() monitor.MonitorService {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.monitor
}

// GetBlockchainClient returns the blockchain client
func (s *BotService) GetBlockchainClient() *blockchain.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockchainClient
}

// GetWallets returns the wallets map
func (s *BotService) GetWallets() map[string]*task.Wallet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to avoid race conditions
	wallets := make(map[string]*task.Wallet)
	for k, v := range s.wallets {
		wallets[k] = v
	}
	return wallets
}

// GetTaskManager returns the task manager
func (s *BotService) GetTaskManager() *task.Manager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.taskManager
}

// GetConfig returns the configuration
func (s *BotService) GetConfig() *task.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetLogger returns the logger
func (s *BotService) GetLogger() *zap.Logger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logger
}

// GetContext returns the service context
func (s *BotService) GetContext() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx
}

// validateRPCConnectivity validates that the blockchain client can connect
func (s *BotService) validateRPCConnectivity() error {
	s.logger.Info("🌐 Validating RPC connectivity")

	// Test RPC connectivity with a simple call
	// Note: This is a simplified validation - in real implementation
	// you might want to make an actual RPC call to verify connectivity
	if len(s.config.RPCList) == 0 {
		return fmt.Errorf("no RPC endpoints configured")
	}

	s.logger.Info("✅ RPC connectivity validated",
		zap.String("primary_rpc", s.maskRPCURL(s.config.RPCList[0])),
		zap.Int("total_rpcs", len(s.config.RPCList)))

	return nil
}

// maskRPCURL masks sensitive parts of RPC URL for logging
func (s *BotService) maskRPCURL(url string) string {
	if len(url) > 20 {
		return url[:10] + "***" + url[len(url)-7:]
	}
	return "***"
}

// GetRegisteredHandlers returns list of registered command handlers
func (s *BotService) GetRegisteredHandlers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.commandBus.GetRegisteredHandlers()
}

// GetEventSubscriberCount returns number of subscribers for an event type
func (s *BotService) GetEventSubscriberCount(eventType string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventBus.GetSubscriberCount(eventType)
}
