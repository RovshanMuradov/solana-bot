// internal/bot/events.go
package bot

import (
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TradingEvent представляет событие в торговой системе
type TradingEvent interface {
	GetType() string
	GetTimestamp() time.Time
	GetUserID() string
}

// TaskExecutedEvent событие выполнения задачи
type TaskExecutedEvent struct {
	TaskID      int       `json:"task_id"`
	TaskName    string    `json:"task_name"`
	TokenMint   string    `json:"token_mint"`
	TxSignature string    `json:"tx_signature"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	UserID      string    `json:"user_id"`
	Timestamp   time.Time `json:"timestamp"`
}

func (e TaskExecutedEvent) GetType() string {
	return "task_executed"
}

func (e TaskExecutedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e TaskExecutedEvent) GetUserID() string {
	return e.UserID
}

// PositionUpdatedEvent событие обновления позиции
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

func (e PositionUpdatedEvent) GetType() string {
	return "position_updated"
}

func (e PositionUpdatedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e PositionUpdatedEvent) GetUserID() string {
	return e.UserID
}

// PositionCreatedEvent событие создания новой позиции
type PositionCreatedEvent struct {
	TaskID       int       `json:"task_id"`
	TokenMint    string    `json:"token_mint"`
	TokenSymbol  string    `json:"token_symbol"`
	EntryPrice   float64   `json:"entry_price"`
	TokenBalance uint64    `json:"token_balance"`
	AmountSol    float64   `json:"amount_sol"`
	TxSignature  string    `json:"tx_signature"`
	UserID       string    `json:"user_id"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e PositionCreatedEvent) GetType() string {
	return "position_created"
}

func (e PositionCreatedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e PositionCreatedEvent) GetUserID() string {
	return e.UserID
}

// SellCompletedEvent событие завершения продажи
type SellCompletedEvent struct {
	TokenMint   string    `json:"token_mint"`
	AmountSold  float64   `json:"amount_sold"`
	SolReceived float64   `json:"sol_received"`
	TxSignature string    `json:"tx_signature"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	UserID      string    `json:"user_id"`
	Timestamp   time.Time `json:"timestamp"`
}

func (e SellCompletedEvent) GetType() string {
	return "sell_completed"
}

func (e SellCompletedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e SellCompletedEvent) GetUserID() string {
	return e.UserID
}

// MonitoringSessionStartedEvent событие запуска мониторинг сессии
type MonitoringSessionStartedEvent struct {
	TokenMint    string    `json:"token_mint"`
	InitialPrice float64   `json:"initial_price"`
	TokenAmount  float64   `json:"token_amount"`
	UserID       string    `json:"user_id"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e MonitoringSessionStartedEvent) GetType() string {
	return "monitoring_session_started"
}

func (e MonitoringSessionStartedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e MonitoringSessionStartedEvent) GetUserID() string {
	return e.UserID
}

// MonitoringSessionStoppedEvent событие остановки мониторинг сессии
type MonitoringSessionStoppedEvent struct {
	TokenMint string    `json:"token_mint"`
	Reason    string    `json:"reason"`
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e MonitoringSessionStoppedEvent) GetType() string {
	return "monitoring_session_stopped"
}

func (e MonitoringSessionStoppedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e MonitoringSessionStoppedEvent) GetUserID() string {
	return e.UserID
}

// EventHandler интерфейс для обработчиков событий
type EventHandler interface {
	Handle(event TradingEvent) error
	CanHandle(event TradingEvent) bool
}

// EventSubscriber интерфейс для подписчиков на события
type EventSubscriber interface {
	OnEvent(event TradingEvent)
	GetSubscribedEventTypes() []string
}

// EventBus шина событий
type EventBus struct {
	handlers    map[reflect.Type][]EventHandler
	subscribers map[string][]EventSubscriber // event_type -> subscribers
	logger      *zap.Logger
	mu          sync.RWMutex
}

// NewEventBus создает новую шину событий
func NewEventBus(logger *zap.Logger) *EventBus {
	return &EventBus{
		handlers:    make(map[reflect.Type][]EventHandler),
		subscribers: make(map[string][]EventSubscriber),
		logger:      logger.Named("event_bus"),
	}
}

// RegisterHandler регистрирует обработчик для типа события
func (bus *EventBus) RegisterHandler(eventType TradingEvent, handler EventHandler) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	eventReflectType := reflect.TypeOf(eventType)
	bus.handlers[eventReflectType] = append(bus.handlers[eventReflectType], handler)

	bus.logger.Info("Event handler registered",
		zap.String("event_type", eventType.GetType()),
		zap.String("handler", reflect.TypeOf(handler).String()))
}

// Subscribe подписывает подписчика на события
func (bus *EventBus) Subscribe(subscriber EventSubscriber) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	for _, eventType := range subscriber.GetSubscribedEventTypes() {
		bus.subscribers[eventType] = append(bus.subscribers[eventType], subscriber)
		bus.logger.Info("Subscriber registered",
			zap.String("event_type", eventType),
			zap.String("subscriber", reflect.TypeOf(subscriber).String()))
	}
}

// Publish публикует событие
func (bus *EventBus) Publish(event TradingEvent) {
	bus.mu.RLock()

	// Найти обработчики по типу
	eventType := reflect.TypeOf(event)
	handlers := bus.handlers[eventType]

	// Найти подписчиков по имени типа
	subscribers := bus.subscribers[event.GetType()]

	bus.mu.RUnlock()

	bus.logger.Info("Publishing event",
		zap.String("event_type", event.GetType()),
		zap.String("user_id", event.GetUserID()),
		zap.Int("handlers", len(handlers)),
		zap.Int("subscribers", len(subscribers)))

	// Обработка через handlers
	for _, handler := range handlers {
		if handler.CanHandle(event) {
			go func(h EventHandler) {
				if err := h.Handle(event); err != nil {
					bus.logger.Error("Event handler failed",
						zap.String("event_type", event.GetType()),
						zap.String("handler", reflect.TypeOf(h).String()),
						zap.Error(err))
				}
			}(handler)
		}
	}

	// Уведомление подписчиков
	for _, subscriber := range subscribers {
		go func(s EventSubscriber) {
			defer func() {
				if r := recover(); r != nil {
					bus.logger.Error("Event subscriber panic",
						zap.String("event_type", event.GetType()),
						zap.String("subscriber", reflect.TypeOf(s).String()),
						zap.Any("panic", r))
				}
			}()
			s.OnEvent(event)
		}(subscriber)
	}
}

// GetSubscriberCount возвращает количество подписчиков для типа события
func (bus *EventBus) GetSubscriberCount(eventType string) int {
	bus.mu.RLock()
	defer bus.mu.RUnlock()
	return len(bus.subscribers[eventType])
}

// GetHandlerCount возвращает количество обработчиков для типа события
func (bus *EventBus) GetHandlerCount(eventType TradingEvent) int {
	bus.mu.RLock()
	defer bus.mu.RUnlock()
	return len(bus.handlers[reflect.TypeOf(eventType)])
}
