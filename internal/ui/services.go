package ui

import (
	"context"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/bot"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

// ServiceProvider provides access to real bot services for UI screens
type ServiceProvider interface {
	GetTaskManager() *task.Manager
	GetBlockchainClient() *blockchain.Client
	GetWallets() map[string]*task.Wallet
	GetLogger() *zap.Logger
	GetConfig() *task.Config
	GetContext() context.Context

	// Command/Event system
	GetCommandBus() *bot.CommandBus
	GetEventBus() *bot.EventBus
	GetTradingService() *bot.TradingService

	// Monitoring session management (deprecated - use TradingService instead)
	AddMonitoringSession(tokenMint string, session *monitor.MonitoringSession)
	GetMonitoringSession(tokenMint string) (*monitor.MonitoringSession, bool)
	GetAllMonitoringSessions() map[string]*monitor.MonitoringSession
	RemoveMonitoringSession(tokenMint string)
}

// RealServiceProvider implements ServiceProvider with real services
type RealServiceProvider struct {
	taskManager      *task.Manager
	blockchainClient *blockchain.Client
	wallets          map[string]*task.Wallet
	logger           *zap.Logger
	config           *task.Config
	context          context.Context
	tradingService   *bot.TradingService

	// Legacy monitoring sessions (deprecated)
	sessions map[string]*monitor.MonitoringSession
}

// NewRealServiceProvider creates a new real service provider
func NewRealServiceProvider(
	ctx context.Context,
	config *task.Config,
	logger *zap.Logger,
	taskManager *task.Manager,
	blockchainClient *blockchain.Client,
	wallets map[string]*task.Wallet,
	tradingService *bot.TradingService,
) ServiceProvider {
	return &RealServiceProvider{
		taskManager:      taskManager,
		blockchainClient: blockchainClient,
		wallets:          wallets,
		logger:           logger.Named("ui_service_provider"),
		config:           config,
		context:          ctx,
		tradingService:   tradingService,
		sessions:         make(map[string]*monitor.MonitoringSession),
	}
}

// GetTaskManager returns the task manager
func (p *RealServiceProvider) GetTaskManager() *task.Manager {
	return p.taskManager
}

// GetBlockchainClient returns the blockchain client
func (p *RealServiceProvider) GetBlockchainClient() *blockchain.Client {
	return p.blockchainClient
}

// GetWallets returns the wallets map
func (p *RealServiceProvider) GetWallets() map[string]*task.Wallet {
	return p.wallets
}

// GetLogger returns the logger
func (p *RealServiceProvider) GetLogger() *zap.Logger {
	return p.logger
}

// GetConfig returns the config
func (p *RealServiceProvider) GetConfig() *task.Config {
	return p.config
}

// GetContext returns the context
func (p *RealServiceProvider) GetContext() context.Context {
	return p.context
}

// GetCommandBus returns the command bus from trading service
func (p *RealServiceProvider) GetCommandBus() *bot.CommandBus {
	return p.tradingService.GetCommandBus()
}

// GetEventBus returns the event bus from trading service
func (p *RealServiceProvider) GetEventBus() *bot.EventBus {
	return p.tradingService.GetEventBus()
}

// GetTradingService returns the trading service
func (p *RealServiceProvider) GetTradingService() *bot.TradingService {
	return p.tradingService
}

// Legacy monitoring session methods (deprecated - use TradingService instead)

// AddMonitoringSession adds a monitoring session (deprecated)
func (p *RealServiceProvider) AddMonitoringSession(tokenMint string, session *monitor.MonitoringSession) {
	p.sessions[tokenMint] = session
}

// GetMonitoringSession gets a monitoring session (deprecated)
func (p *RealServiceProvider) GetMonitoringSession(tokenMint string) (*monitor.MonitoringSession, bool) {
	session, exists := p.sessions[tokenMint]
	return session, exists
}

// GetAllMonitoringSessions returns all monitoring sessions (deprecated)
func (p *RealServiceProvider) GetAllMonitoringSessions() map[string]*monitor.MonitoringSession {
	return p.sessions
}

// RemoveMonitoringSession removes a monitoring session (deprecated)
func (p *RealServiceProvider) RemoveMonitoringSession(tokenMint string) {
	delete(p.sessions, tokenMint)
}
