// internal/bot/events_test.go
package bot

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// MockEventHandler для тестирования
type MockEventHandler struct {
	handled []TradingEvent
	errors  map[string]error
	mu      sync.Mutex
}

func NewMockEventHandler() *MockEventHandler {
	return &MockEventHandler{
		handled: make([]TradingEvent, 0),
		errors:  make(map[string]error),
	}
}

func (h *MockEventHandler) Handle(event TradingEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handled = append(h.handled, event)
	if err, exists := h.errors[event.GetType()]; exists {
		return err
	}
	return nil
}

func (h *MockEventHandler) CanHandle(event TradingEvent) bool {
	return true
}

func (h *MockEventHandler) SetError(eventType string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errors[eventType] = err
}

func (h *MockEventHandler) GetHandledEvents() []TradingEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]TradingEvent(nil), h.handled...)
}

// MockEventSubscriber для тестирования
type MockEventSubscriber struct {
	received []TradingEvent
	types    []string
	mu       sync.Mutex
}

func NewMockEventSubscriber(types []string) *MockEventSubscriber {
	return &MockEventSubscriber{
		received: make([]TradingEvent, 0),
		types:    types,
	}
}

func (s *MockEventSubscriber) OnEvent(event TradingEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received = append(s.received, event)
}

func (s *MockEventSubscriber) GetSubscribedEventTypes() []string {
	return s.types
}

func (s *MockEventSubscriber) GetReceivedEvents() []TradingEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TradingEvent(nil), s.received...)
}

func TestTaskExecutedEvent_Methods(t *testing.T) {
	timestamp := time.Now()
	event := TaskExecutedEvent{
		TaskID:      1,
		TaskName:    "test_task",
		TokenMint:   "test_mint",
		TxSignature: "test_tx",
		Success:     true,
		UserID:      "test_user",
		Timestamp:   timestamp,
	}

	if event.GetType() != "task_executed" {
		t.Errorf("Expected type 'task_executed', got '%s'", event.GetType())
	}

	if event.GetUserID() != "test_user" {
		t.Errorf("Expected user_id 'test_user', got '%s'", event.GetUserID())
	}

	if !event.GetTimestamp().Equal(timestamp) {
		t.Errorf("Expected timestamp %v, got %v", timestamp, event.GetTimestamp())
	}
}

func TestPositionUpdatedEvent_Methods(t *testing.T) {
	timestamp := time.Now()
	event := PositionUpdatedEvent{
		TokenMint:    "test_mint",
		TokenSymbol:  "TEST",
		CurrentPrice: 1.5,
		EntryPrice:   1.0,
		PnLPercent:   50.0,
		PnLSol:       0.5,
		Amount:       100.0,
		UserID:       "test_user",
		Timestamp:    timestamp,
	}

	if event.GetType() != "position_updated" {
		t.Errorf("Expected type 'position_updated', got '%s'", event.GetType())
	}

	if event.GetUserID() != "test_user" {
		t.Errorf("Expected user_id 'test_user', got '%s'", event.GetUserID())
	}

	if !event.GetTimestamp().Equal(timestamp) {
		t.Errorf("Expected timestamp %v, got %v", timestamp, event.GetTimestamp())
	}
}

func TestEventBus_RegisterHandler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewEventBus(logger)
	handler := NewMockEventHandler()

	// Регистрируем обработчик
	bus.RegisterHandler(TaskExecutedEvent{}, handler)

	count := bus.GetHandlerCount(TaskExecutedEvent{})
	if count != 1 {
		t.Errorf("Expected 1 registered handler, got %d", count)
	}
}

func TestEventBus_Subscribe(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewEventBus(logger)
	subscriber := NewMockEventSubscriber([]string{"task_executed", "position_updated"})

	// Подписываем
	bus.Subscribe(subscriber)

	count1 := bus.GetSubscriberCount("task_executed")
	count2 := bus.GetSubscriberCount("position_updated")

	if count1 != 1 {
		t.Errorf("Expected 1 subscriber for 'task_executed', got %d", count1)
	}

	if count2 != 1 {
		t.Errorf("Expected 1 subscriber for 'position_updated', got %d", count2)
	}
}

func TestEventBus_Publish_Handler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewEventBus(logger)
	handler := NewMockEventHandler()

	// Регистрируем обработчик
	bus.RegisterHandler(TaskExecutedEvent{}, handler)

	// Создаем событие
	event := TaskExecutedEvent{
		TaskID:      1,
		TaskName:    "test_task",
		TokenMint:   "test_mint",
		TxSignature: "test_tx",
		Success:     true,
		UserID:      "test_user",
		Timestamp:   time.Now(),
	}

	// Публикуем событие
	bus.Publish(event)

	// Ждем обработки (события обрабатываются в горутинах)
	time.Sleep(100 * time.Millisecond)

	// Проверяем, что событие было обработано
	handled := handler.GetHandledEvents()
	if len(handled) != 1 {
		t.Errorf("Expected 1 handled event, got %d", len(handled))
	}

	if handled[0].GetType() != "task_executed" {
		t.Errorf("Expected 'task_executed' event, got '%s'", handled[0].GetType())
	}
}

func TestEventBus_Publish_Subscriber(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewEventBus(logger)
	subscriber := NewMockEventSubscriber([]string{"task_executed"})

	// Подписываем
	bus.Subscribe(subscriber)

	// Создаем событие
	event := TaskExecutedEvent{
		TaskID:      1,
		TaskName:    "test_task",
		TokenMint:   "test_mint",
		TxSignature: "test_tx",
		Success:     true,
		UserID:      "test_user",
		Timestamp:   time.Now(),
	}

	// Публикуем событие
	bus.Publish(event)

	// Ждем обработки (события обрабатываются в горутинах)
	time.Sleep(100 * time.Millisecond)

	// Проверяем, что событие было получено
	received := subscriber.GetReceivedEvents()
	if len(received) != 1 {
		t.Errorf("Expected 1 received event, got %d", len(received))
	}

	if received[0].GetType() != "task_executed" {
		t.Errorf("Expected 'task_executed' event, got '%s'", received[0].GetType())
	}
}

func TestEventBus_Publish_MultipleSubscribers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewEventBus(logger)

	subscriber1 := NewMockEventSubscriber([]string{"task_executed"})
	subscriber2 := NewMockEventSubscriber([]string{"task_executed", "position_updated"})

	// Подписываем обоих
	bus.Subscribe(subscriber1)
	bus.Subscribe(subscriber2)

	// Создаем событие
	event := TaskExecutedEvent{
		TaskID:      1,
		TaskName:    "test_task",
		TokenMint:   "test_mint",
		TxSignature: "test_tx",
		Success:     true,
		UserID:      "test_user",
		Timestamp:   time.Now(),
	}

	// Публикуем событие
	bus.Publish(event)

	// Ждем обработки
	time.Sleep(100 * time.Millisecond)

	// Проверяем, что оба подписчика получили событие
	received1 := subscriber1.GetReceivedEvents()
	received2 := subscriber2.GetReceivedEvents()

	if len(received1) != 1 {
		t.Errorf("Expected 1 received event for subscriber1, got %d", len(received1))
	}

	if len(received2) != 1 {
		t.Errorf("Expected 1 received event for subscriber2, got %d", len(received2))
	}
}

func TestEventBus_Publish_NoSubscribers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewEventBus(logger)

	// НЕ подписываем никого

	// Создаем событие
	event := TaskExecutedEvent{
		TaskID:      1,
		TaskName:    "test_task",
		TokenMint:   "test_mint",
		TxSignature: "test_tx",
		Success:     true,
		UserID:      "test_user",
		Timestamp:   time.Now(),
	}

	// Публикуем событие (не должно паниковать)
	bus.Publish(event)

	// Проверяем счетчики
	count := bus.GetSubscriberCount("task_executed")
	if count != 0 {
		t.Errorf("Expected 0 subscribers, got %d", count)
	}
}
