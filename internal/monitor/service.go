// internal/monitor/service.go
package monitor

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

// MonitorService defines the interface for monitoring service
type MonitorService interface {
	// CreateMonitoringSession creates a new monitoring session
	CreateMonitoringSession(ctx context.Context, req *CreateSessionRequest) (*MonitoringSession, error)

	// StopMonitoringSession stops a monitoring session
	StopMonitoringSession(tokenMint string) error

	// GetSession retrieves a monitoring session
	GetSession(tokenMint string) (*MonitoringSession, bool)

	// GetAllSessions returns all active sessions
	GetAllSessions() map[string]*MonitoringSession

	// RemoveSession removes a session from management
	RemoveSession(tokenMint string)

	// Shutdown gracefully shuts down all monitoring sessions
	Shutdown(ctx context.Context) error
}

// CreateSessionRequest contains parameters for creating a monitoring session
type CreateSessionRequest struct {
	Task         *task.Task    // Trading task data
	EntryPrice   float64       // Entry price for the position
	TokenBalance uint64        // Raw token balance in smallest units
	Wallet       *task.Wallet  // Wallet for the session
	DEXName      string        // DEX adapter name (e.g., "snipe", "pumpfun")
	Interval     time.Duration // Monitoring interval
	UserID       string        // User ID for event publishing
}

// CreateSessionResponse contains the result of session creation
type CreateSessionResponse struct {
	Session   *MonitoringSession
	TokenMint string
	Success   bool
	Error     string
}

// MonitorServiceImpl implements the MonitorService interface
type MonitorServiceImpl struct {
	sessions         map[string]*MonitoringSession
	mutex            sync.RWMutex
	logger           *zap.Logger
	blockchainClient *blockchain.Client
	eventBus         EventBus
	priceThrottler   *PriceThrottler // Phase 2: Price update throttling
}

// EventBus defines the interface for event publishing
type EventBus interface {
	Publish(event interface{})
}

// MonitorServiceConfig configuration for MonitorService
type MonitorServiceConfig struct {
	Logger           *zap.Logger
	BlockchainClient *blockchain.Client
	EventBus         EventBus
	UIMessageChannel chan tea.Msg // Phase 2: UI message channel for throttling
}

// NewMonitorService creates a new monitor service
func NewMonitorService(config *MonitorServiceConfig) MonitorService {
	ms := &MonitorServiceImpl{
		sessions:         make(map[string]*MonitoringSession),
		logger:           config.Logger.Named("monitor_service"),
		blockchainClient: config.BlockchainClient,
		eventBus:         config.EventBus,
	}

	// Phase 2: Initialize price throttler with O3-recommended 150ms interval
	if config.UIMessageChannel != nil {
		ms.priceThrottler = NewPriceThrottler(
			150*time.Millisecond, // O3 recommendation for optimal UI performance
			config.UIMessageChannel,
			ms.logger,
		)

		// Start throttler flush goroutine
		go ms.runThrottlerFlush()

		ms.logger.Info("PriceThrottler initialized",
			zap.Duration("interval", 150*time.Millisecond))
	} else {
		ms.logger.Warn("UIMessageChannel not provided, price throttling disabled")
	}

	return ms
}

// CreateMonitoringSession creates a new monitoring session
func (ms *MonitorServiceImpl) CreateMonitoringSession(ctx context.Context, req *CreateSessionRequest) (*MonitoringSession, error) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	ms.logger.Info("📊 Creating monitoring session",
		zap.String("token", req.Task.TokenMint),
		zap.String("wallet", req.Task.WalletName),
		zap.Float64("entry_price", req.EntryPrice),
		zap.Uint64("balance", req.TokenBalance))

	// Validate request
	if err := ms.validateCreateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid create session request: %w", err)
	}

	// Check if session already exists
	if _, exists := ms.sessions[req.Task.TokenMint]; exists {
		return nil, fmt.Errorf("monitoring session already exists for token %s", req.Task.TokenMint)
	}

	// Create DEX adapter for monitoring
	dexAdapter, err := dex.GetDEXByName(req.DEXName, ms.blockchainClient, req.Wallet, ms.logger)
	if err != nil {
		ms.logger.Error("Failed to create DEX adapter for monitoring",
			zap.String("dex", req.DEXName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create DEX adapter for monitoring: %w", err)
	}

	ms.logger.Info("DEX adapter created for monitoring",
		zap.String("dex", req.DEXName),
		zap.String("token", req.Task.TokenMint))

	// Set default interval if not provided
	interval := req.Interval
	if interval == 0 {
		interval = 5 * time.Second
	}

	// Create monitoring session config
	sessionConfig := &SessionConfig{
		Task:            req.Task,
		TokenBalance:    req.TokenBalance,
		InitialPrice:    req.EntryPrice,
		DEX:             dexAdapter,
		Logger:          ms.logger.Named("session"),
		MonitorInterval: interval,
	}

	// Create and start monitoring session
	session := NewMonitoringSession(ctx, sessionConfig)
	if err := session.Start(); err != nil {
		ms.logger.Error("Failed to start monitoring session", zap.Error(err))
		return nil, fmt.Errorf("failed to start monitoring session: %w", err)
	}

	// Add session to management
	ms.sessions[req.Task.TokenMint] = session

	// Start event publishing goroutine
	go ms.startEventPublishing(ctx, session, req)

	// Publish monitoring session started event
	ms.publishSessionStartedEvent(req)

	ms.logger.Info("✅ Monitoring session created successfully",
		zap.String("token", req.Task.TokenMint),
		zap.Float64("entry_price", req.EntryPrice),
		zap.Uint64("balance", req.TokenBalance))

	return session, nil
}

// StopMonitoringSession stops a monitoring session
func (ms *MonitorServiceImpl) StopMonitoringSession(tokenMint string) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	session, exists := ms.sessions[tokenMint]
	if !exists {
		return fmt.Errorf("monitoring session not found for token %s", tokenMint)
	}

	ms.logger.Info("🛑 Stopping monitoring session", zap.String("token", tokenMint))

	// Stop the session
	session.Stop()

	// Remove from management
	delete(ms.sessions, tokenMint)

	// Publish session stopped event
	ms.publishSessionStoppedEvent(tokenMint, "manual_stop")

	ms.logger.Info("✅ Monitoring session stopped", zap.String("token", tokenMint))
	return nil
}

// GetSession retrieves a monitoring session
func (ms *MonitorServiceImpl) GetSession(tokenMint string) (*MonitoringSession, bool) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	session, exists := ms.sessions[tokenMint]
	return session, exists
}

// GetAllSessions returns all active sessions
func (ms *MonitorServiceImpl) GetAllSessions() map[string]*MonitoringSession {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	// Create a copy to avoid race conditions
	sessions := make(map[string]*MonitoringSession)
	for k, v := range ms.sessions {
		sessions[k] = v
	}
	return sessions
}

// RemoveSession removes a session from management without stopping it
func (ms *MonitorServiceImpl) RemoveSession(tokenMint string) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if _, exists := ms.sessions[tokenMint]; exists {
		delete(ms.sessions, tokenMint)
		ms.logger.Info("Monitoring session removed from management", zap.String("token", tokenMint))
	}
}

// Shutdown gracefully shuts down all monitoring sessions
func (ms *MonitorServiceImpl) Shutdown(ctx context.Context) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	ms.logger.Info("Shutting down monitor service", zap.Int("active_sessions", len(ms.sessions)))

	// Stop all sessions
	for tokenMint, session := range ms.sessions {
		ms.logger.Info("Stopping session during shutdown", zap.String("token", tokenMint))
		session.Stop()
	}

	// Clear sessions map
	ms.sessions = make(map[string]*MonitoringSession)

	ms.logger.Info("Monitor service shutdown completed")
	return nil
}

// validateCreateRequest validates the create session request
func (ms *MonitorServiceImpl) validateCreateRequest(req *CreateSessionRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}
	if req.Task == nil {
		return fmt.Errorf("task cannot be nil")
	}
	if req.Task.TokenMint == "" {
		return fmt.Errorf("token mint cannot be empty")
	}
	if req.Wallet == nil {
		return fmt.Errorf("wallet cannot be nil")
	}
	if req.DEXName == "" {
		return fmt.Errorf("DEX name cannot be empty")
	}
	if req.EntryPrice <= 0 {
		return fmt.Errorf("entry price must be positive")
	}
	return nil
}

// startEventPublishing starts a goroutine to publish monitoring events
func (ms *MonitorServiceImpl) startEventPublishing(ctx context.Context, session *MonitoringSession, req *CreateSessionRequest) {
	ms.logger.Info("Starting event publishing goroutine", zap.String("token", req.Task.TokenMint))

	for {
		select {
		case <-ctx.Done():
			ms.logger.Debug("Context cancelled, stopping event publishing", zap.String("token", req.Task.TokenMint))
			return
		case update, ok := <-session.PriceUpdates():
			if !ok {
				ms.logger.Debug("Price updates channel closed", zap.String("token", req.Task.TokenMint))
				return
			}
			// Phase 2: Use PriceThrottler to prevent UI flooding
			if ms.priceThrottler != nil {
				ms.priceThrottler.SendPriceUpdate(update)
			} else {
				// Fallback to direct publishing if throttler not available
				ms.publishPositionUpdatedEvent(req.Task.TokenMint, update, req.UserID)
			}
		case err, ok := <-session.Err():
			if !ok {
				ms.logger.Debug("Error channel closed", zap.String("token", req.Task.TokenMint))
				return
			}
			ms.logger.Error("Monitoring session error",
				zap.String("token", req.Task.TokenMint),
				zap.Error(err))
		}
	}
}

// publishSessionStartedEvent publishes a monitoring session started event
func (ms *MonitorServiceImpl) publishSessionStartedEvent(req *CreateSessionRequest) {
	if ms.eventBus == nil {
		return
	}

	event := MonitoringSessionStartedEvent{
		TokenMint:    req.Task.TokenMint,
		InitialPrice: req.EntryPrice,
		TokenAmount:  float64(req.TokenBalance) / math.Pow10(6), // Assuming 6 decimals
		UserID:       req.UserID,
		Timestamp:    time.Now(),
	}

	ms.eventBus.Publish(event)
	ms.logger.Debug("Published monitoring session started event",
		zap.String("token", req.Task.TokenMint))
}

// publishPositionUpdatedEvent publishes a position updated event
func (ms *MonitorServiceImpl) publishPositionUpdatedEvent(tokenMint string, update PriceUpdate, userID string) {
	if ms.eventBus == nil {
		return
	}

	event := PositionUpdatedEvent{
		TokenMint:    tokenMint,
		TokenSymbol:  ms.getTokenSymbol(tokenMint),
		CurrentPrice: update.Current,
		EntryPrice:   update.Initial,
		PnLPercent:   update.Percent,
		PnLSol:       (update.Current - update.Initial) * update.Tokens,
		Amount:       update.Tokens,
		UserID:       userID,
		Timestamp:    time.Now(),
	}

	ms.eventBus.Publish(event)
	ms.logger.Debug("Published position updated event",
		zap.String("token", tokenMint),
		zap.Float64("current_price", update.Current),
		zap.Float64("pnl_percent", update.Percent))
}

// publishSessionStoppedEvent publishes a monitoring session stopped event
func (ms *MonitorServiceImpl) publishSessionStoppedEvent(tokenMint, reason string) {
	if ms.eventBus == nil {
		return
	}

	event := MonitoringSessionStoppedEvent{
		TokenMint: tokenMint,
		Reason:    reason,
		UserID:    "system",
		Timestamp: time.Now(),
	}

	ms.eventBus.Publish(event)
	ms.logger.Debug("Published monitoring session stopped event",
		zap.String("token", tokenMint),
		zap.String("reason", reason))
}

// getTokenSymbol extracts a symbol from token mint (simplified)
func (ms *MonitorServiceImpl) getTokenSymbol(tokenMint string) string {
	if len(tokenMint) >= 8 {
		return tokenMint[:4] + "..." + tokenMint[len(tokenMint)-4:]
	}
	return "TOKEN"
}

// runThrottlerFlush runs periodic flush of pending price updates
func (ms *MonitorServiceImpl) runThrottlerFlush() {
	if ms.priceThrottler == nil {
		return
	}

	ticker := time.NewTicker(100 * time.Millisecond) // Flush more frequently than throttle interval
	defer ticker.Stop()

	ms.logger.Debug("Price throttler flush loop started")

	for range ticker.C {
		ms.priceThrottler.FlushPending()
	}
}

// Event types for monitoring
type MonitoringSessionStartedEvent struct {
	TokenMint    string    `json:"token_mint"`
	InitialPrice float64   `json:"initial_price"`
	TokenAmount  float64   `json:"token_amount"`
	UserID       string    `json:"user_id"`
	Timestamp    time.Time `json:"timestamp"`
}

type PositionUpdatedEvent struct {
	TokenMint    string    `json:"token_mint"`
	TokenSymbol  string    `json:"token_symbol"`
	CurrentPrice float64   `json:"current_price"`
	EntryPrice   float64   `json:"entry_price"`
	PnLPercent   float64   `json:"pnl_percent"`
	PnLSol       float64   `json:"pnl_sol"`
	Amount       float64   `json:"amount"`
	UserID       string    `json:"user_id"`
	Timestamp    time.Time `json:"timestamp"`
}

type MonitoringSessionStoppedEvent struct {
	TokenMint string    `json:"token_mint"`
	Reason    string    `json:"reason"`
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}
