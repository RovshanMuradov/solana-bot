package domain

import (
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap/zapcore"
)

// EventType represents the type of domain event
type EventType int

const (
	EventTaskCreated EventType = iota
	EventTaskExecuted
	EventTaskFailed
	EventPriceTick
	EventOrderFilled
	EventPnLUpdate
	EventLog
	EventWorkerStarted
	EventWorkerStopped
	EventMonitoringStarted
	EventMonitoringStopped
)

// Event represents a domain event
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// NewEvent creates a new domain event
func NewEvent(eventType EventType, data interface{}) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// Event data structures

// TaskCreatedData contains data for task creation events
type TaskCreatedData struct {
	Task *task.Task `json:"task"`
}

// TaskExecutedData contains data for task execution events
type TaskExecutedData struct {
	TaskID   int           `json:"task_id"`
	Task     *task.Task    `json:"task"`
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
	TxHash   string        `json:"tx_hash,omitempty"`
	Duration time.Duration `json:"duration"`
}

// PriceTickData contains real-time price information
type PriceTickData struct {
	TokenMint   string    `json:"token_mint"`
	Price       float64   `json:"price"`
	PriceChange float64   `json:"price_change"`
	Volume24h   float64   `json:"volume_24h,omitempty"`
	MarketCap   float64   `json:"market_cap,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// OrderFilledData contains order execution information
type OrderFilledData struct {
	TaskID    int     `json:"task_id"`
	TokenMint string  `json:"token_mint"`
	Operation string  `json:"operation"` // buy/sell
	Amount    float64 `json:"amount"`
	Price     float64 `json:"price"`
	TotalCost float64 `json:"total_cost"`
	TxHash    string  `json:"tx_hash"`
	Slippage  float64 `json:"slippage"`
}

// PnLUpdateData contains profit and loss information
type PnLUpdateData struct {
	TaskID        int             `json:"task_id"`
	TokenMint     string          `json:"token_mint"`
	PnL           model.PnLResult `json:"pnl"`
	CurrentPrice  float64         `json:"current_price"`
	InitialPrice  float64         `json:"initial_price"`
	TokenBalance  float64         `json:"token_balance"`
	PercentChange float64         `json:"percent_change"`
}

// LogData contains structured log information
type LogData struct {
	Level     zapcore.Level          `json:"level"`
	Message   string                 `json:"message"`
	Component string                 `json:"component,omitempty"`
	TaskID    int                    `json:"task_id,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// WorkerStartedData contains worker startup information
type WorkerStartedData struct {
	WorkerID  int `json:"worker_id"`
	TaskCount int `json:"task_count"`
}

// WorkerStoppedData contains worker shutdown information
type WorkerStoppedData struct {
	WorkerID       int           `json:"worker_id"`
	TasksCompleted int           `json:"tasks_completed"`
	Duration       time.Duration `json:"duration"`
	Error          string        `json:"error,omitempty"`
}

// MonitoringStartedData contains monitoring session startup information
type MonitoringStartedData struct {
	SessionID    string  `json:"session_id"`
	TokenMint    string  `json:"token_mint"`
	InitialPrice float64 `json:"initial_price"`
	TokenBalance float64 `json:"token_balance"`
	TaskID       int     `json:"task_id"`
}

// MonitoringStoppedData contains monitoring session shutdown information
type MonitoringStoppedData struct {
	SessionID  string        `json:"session_id"`
	TokenMint  string        `json:"token_mint"`
	FinalPrice float64       `json:"final_price"`
	FinalPnL   float64       `json:"final_pnl"`
	Duration   time.Duration `json:"duration"`
	Reason     string        `json:"reason"` // user_exit, auto_sell, error
}
