package ui

import (
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
)

// Additional events for real trading integration

// TaskStartedMsg represents a task that has started execution
type TaskStartedMsg struct {
	TaskID    int
	TaskName  string
	TokenMint string
	Operation string
	AmountSol float64
	Wallet    string
}

// TaskCompletedMsg represents a completed task with transaction details
type TaskCompletedMsg struct {
	TaskID      int
	TaskName    string
	TokenMint   string
	TxSignature string
	Success     bool
	Error       string
}

// PositionCreatedMsg represents a new position created from successful buy
type PositionCreatedMsg struct {
	TaskID       int
	TokenMint    string
	TokenSymbol  string
	EntryPrice   float64
	TokenBalance uint64
	AmountSol    float64
	TxSignature  string
}

// RealPriceUpdateMsg represents real-time price updates with token mint
type RealPriceUpdateMsg struct {
	TokenMint string
	Update    monitor.PriceUpdate
}

// MonitoringSessionStartedMsg represents a new monitoring session
type MonitoringSessionStartedMsg struct {
	TokenMint    string
	InitialPrice float64
	TokenAmount  float64
}

// MonitoringSessionStoppedMsg represents a stopped monitoring session
type MonitoringSessionStoppedMsg struct {
	TokenMint string
	Reason    string
}

// SellOrderMsg represents a sell order request
type SellOrderMsg struct {
	TokenMint     string
	PercentToSell float64
	RequestedBy   string
}

// SellCompletedMsg represents a completed sell transaction
type SellCompletedMsg struct {
	TokenMint   string
	AmountSold  float64
	SolReceived float64
	TxSignature string
	Success     bool
	Error       string
}

// PublishTaskStarted publishes a task started event
func PublishTaskStarted(msg TaskStartedMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}

// PublishTaskCompleted publishes a task completed event
func PublishTaskCompleted(msg TaskCompletedMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}

// PublishPositionCreated publishes a position created event
func PublishPositionCreated(msg PositionCreatedMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}

// PublishRealPriceUpdate publishes a real price update event
func PublishRealPriceUpdate(msg RealPriceUpdateMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}

// PublishMonitoringSessionStarted publishes a monitoring session started event
func PublishMonitoringSessionStarted(msg MonitoringSessionStartedMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}

// PublishMonitoringSessionStopped publishes a monitoring session stopped event
func PublishMonitoringSessionStopped(msg MonitoringSessionStoppedMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}

// PublishSellOrder publishes a sell order event
func PublishSellOrder(msg SellOrderMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}

// PublishSellCompleted publishes a sell completed event
func PublishSellCompleted(msg SellCompletedMsg) {
	select {
	case Bus <- msg:
	default:
		// Bus is full, drop the event
	}
}
