package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/domain"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"go.uber.org/zap/zapcore"
)

// Tea message types for UI communication

// RouterMsg represents navigation between screens
type RouterMsg struct {
	To Route
}

// DomainEventMsg wraps domain events for the UI
type DomainEventMsg struct {
	Event domain.Event
}

// PriceUpdateMsg represents real-time price updates
type PriceUpdateMsg struct {
	Update monitor.PriceUpdate
}

// LogMsg represents log messages
type LogMsg struct {
	Level   zapcore.Level
	Message string
	Fields  map[string]interface{}
}

// TaskCreatedMsg represents a newly created task
type TaskCreatedMsg struct {
	TaskID   int
	TaskName string
}

// TaskExecutedMsg represents task execution result
type TaskExecutedMsg struct {
	TaskID  int
	Success bool
	Error   string
	TxHash  string
}

// MonitoringStatusMsg represents monitoring session status
type MonitoringStatusMsg struct {
	SessionID string
	Active    bool
	TokenMint string
}

// ErrorMsg represents error conditions
type ErrorMsg struct {
	Error error
	Title string
}

// SuccessMsg represents success conditions
type SuccessMsg struct {
	Message string
	Title   string
}

// WindowResizeMsg represents terminal window resize
type WindowResizeMsg struct {
	Width  int
	Height int
}

// KeyMapMsg represents keymap changes
type KeyMapMsg struct {
	KeyMap map[string]string
}

// Event Bus for UI communication
var (
	// Bus is the global event bus for UI communication
	Bus = make(chan tea.Msg, 1024)
)

// PublishEvent publishes a domain event to the UI bus
func PublishEvent(event domain.Event) {
	select {
	case Bus <- DomainEventMsg{Event: event}:
	default:
		// Bus is full, drop the event
	}
}

// PublishPriceUpdate publishes a price update to the UI bus
func PublishPriceUpdate(update monitor.PriceUpdate) {
	select {
	case Bus <- PriceUpdateMsg{Update: update}:
	default:
		// Bus is full, drop the update
	}
}

// PublishLog publishes a log message to the UI bus
func PublishLog(level zapcore.Level, message string, fields map[string]interface{}) {
	select {
	case Bus <- LogMsg{Level: level, Message: message, Fields: fields}:
	default:
		// Bus is full, drop the log
	}
}

// PublishError publishes an error message to the UI bus
func PublishError(err error, title string) {
	select {
	case Bus <- ErrorMsg{Error: err, Title: title}:
	default:
		// Bus is full, drop the error
	}
}

// PublishSuccess publishes a success message to the UI bus
func PublishSuccess(message, title string) {
	select {
	case Bus <- SuccessMsg{Message: message, Title: title}:
	default:
		// Bus is full, drop the success message
	}
}

// ListenBus returns a tea.Cmd that listens to the event bus
func ListenBus() tea.Cmd {
	return func() tea.Msg {
		return <-Bus
	}
}

// Route represents different screens in the application
type Route int

const (
	RouteMainMenu Route = iota
	RouteCreateTask
	RouteTaskList
	RouteMonitor
	RouteSettings
	RouteLogs
)

// String returns the string representation of the route
func (r Route) String() string {
	switch r {
	case RouteMainMenu:
		return "main_menu"
	case RouteCreateTask:
		return "create_task"
	case RouteTaskList:
		return "task_list"
	case RouteMonitor:
		return "monitor"
	case RouteSettings:
		return "settings"
	case RouteLogs:
		return "logs"
	default:
		return "unknown"
	}
}
