package ui

import (
	"context"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
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

	// Monitoring session management
	AddMonitoringSession(tokenMint string, session *monitor.MonitoringSession)
	GetMonitoringSession(tokenMint string) (*monitor.MonitoringSession, bool)
	GetAllMonitoringSessions() map[string]*monitor.MonitoringSession
	RemoveMonitoringSession(tokenMint string)
}
