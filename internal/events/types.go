// internal/events/types.go
package events

import (
	"time"
)

// EventType represents the type of event.
type EventType string

const (
	// Operation events
	OperationStarted   EventType = "operation.started"
	OperationCompleted EventType = "operation.completed"
	OperationFailed    EventType = "operation.failed"

	// Price events
	PriceUpdated EventType = "price.updated"

	// Balance events
	BalanceChanged EventType = "balance.changed"

	// Monitoring events
	MonitoringStarted EventType = "monitoring.started"
	MonitoringStopped EventType = "monitoring.stopped"
)

// Event is the base interface for all events.
type Event interface {
	Type() EventType
	Timestamp() time.Time
}

// BaseEvent provides common fields for all events.
type BaseEvent struct {
	EventType EventType
	EventTime time.Time
}

// Type returns the event type.
func (e BaseEvent) Type() EventType {
	return e.EventType
}

// Timestamp returns when the event occurred.
func (e BaseEvent) Timestamp() time.Time {
	return e.EventTime
}

// OperationStartedEvent is emitted when an operation begins.
type OperationStartedEvent struct {
	BaseEvent
	TaskID     int
	TaskName   string
	Operation  string
	WalletName string
	TokenMint  string
}

// OperationCompletedEvent is emitted when an operation completes successfully.
type OperationCompletedEvent struct {
	BaseEvent
	TaskID     int
	TaskName   string
	Operation  string
	WalletName string
	TokenMint  string
	Result     interface{} // Transaction signature, tokens received, etc.
}

// OperationFailedEvent is emitted when an operation fails.
type OperationFailedEvent struct {
	BaseEvent
	TaskID     int
	TaskName   string
	Operation  string
	WalletName string
	TokenMint  string
	Error      error
}

// PriceUpdatedEvent is emitted when token price changes.
type PriceUpdatedEvent struct {
	BaseEvent
	TokenMint    string
	CurrentPrice float64
	InitialPrice float64
	PriceChange  float64 // Percentage change
	TokenAmount  float64
}

// BalanceChangedEvent is emitted when wallet balance changes.
type BalanceChangedEvent struct {
	BaseEvent
	WalletAddress string
	TokenMint     string
	OldBalance    uint64
	NewBalance    uint64
}

// MonitoringStartedEvent is emitted when price monitoring begins.
type MonitoringStartedEvent struct {
	BaseEvent
	TaskID       int
	TokenMint    string
	InitialPrice float64
	TokenAmount  float64
}

// MonitoringStoppedEvent is emitted when price monitoring ends.
type MonitoringStoppedEvent struct {
	BaseEvent
	TaskID    int
	TokenMint string
	Reason    string // "sold", "exit", "error"
}
