// internal/bot/integration_test.go
package bot

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestCommandEventIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Создаем шины
	commandBus := NewCommandBus(logger)
	eventBus := NewEventBus(logger)

	// Создаем обработчик команд
	tradingHandler := NewExampleTradingHandler(logger, eventBus)

	// Регистрируем обработчики команд
	commandBus.RegisterHandler(ExecuteTaskCommand{}, tradingHandler)
	commandBus.RegisterHandler(SellPositionCommand{}, tradingHandler)

	// Создаем подписчика UI
	uiSubscriber := NewExampleUISubscriber(logger)

	// Подписываем UI на события
	eventBus.Subscribe(uiSubscriber)

	// Проверяем инициализацию
	handlers := commandBus.GetRegisteredHandlers()
	if len(handlers) != 2 {
		t.Errorf("Expected 2 registered handlers, got %d", len(handlers))
	}

	// Тест 1: Выполнение задачи
	t.Run("ExecuteTask", func(t *testing.T) {
		cmd := ExecuteTaskCommand{
			TaskID:    1,
			UserID:    "test_user",
			Timestamp: time.Now(),
		}

		ctx := context.Background()
		err := commandBus.Send(ctx, cmd)
		if err != nil {
			t.Errorf("Failed to execute task: %v", err)
		}

		// Ждем обработки событий
		time.Sleep(150 * time.Millisecond)

		// Проверяем, что UI получил события
		received := uiSubscriber.GetReceivedEvents()
		if len(received) < 2 {
			t.Errorf("Expected at least 2 events, got %d", len(received))
		}

		// Проверяем типы событий
		eventTypes := make(map[string]bool)
		for _, event := range received {
			eventTypes[event.GetType()] = true
		}

		if !eventTypes["task_executed"] {
			t.Error("Expected task_executed event")
		}

		if !eventTypes["position_created"] {
			t.Error("Expected position_created event")
		}
	})

	// Тест 2: Продажа позиции
	t.Run("SellPosition", func(t *testing.T) {
		// Очищаем предыдущие события
		uiSubscriber.received = make([]TradingEvent, 0)

		cmd := SellPositionCommand{
			TokenMint:  "test_token_mint",
			Percentage: 75.0,
			UserID:     "test_user",
			Timestamp:  time.Now(),
		}

		ctx := context.Background()
		err := commandBus.Send(ctx, cmd)
		if err != nil {
			t.Errorf("Failed to sell position: %v", err)
		}

		// Ждем обработки событий
		time.Sleep(200 * time.Millisecond)

		// Проверяем, что UI получил событие продажи
		received := uiSubscriber.GetReceivedEvents()
		if len(received) != 1 {
			t.Errorf("Expected 1 event, got %d", len(received))
		}

		if received[0].GetType() != "sell_completed" {
			t.Errorf("Expected sell_completed event, got %s", received[0].GetType())
		}
	})

	// Тест 3: Обновление позиции
	t.Run("PositionUpdate", func(t *testing.T) {
		// Очищаем предыдущие события
		uiSubscriber.received = make([]TradingEvent, 0)

		event := PositionUpdatedEvent{
			TokenMint:    "test_token_mint",
			TokenSymbol:  "TEST",
			CurrentPrice: 2.0,
			EntryPrice:   1.0,
			PnLPercent:   100.0,
			PnLSol:       1.0,
			Amount:       1000.0,
			UserID:       "test_user",
			Timestamp:    time.Now(),
		}

		eventBus.Publish(event)

		// Ждем обработки событий
		time.Sleep(100 * time.Millisecond)

		// Проверяем, что UI получил событие
		received := uiSubscriber.GetReceivedEvents()
		if len(received) != 1 {
			t.Errorf("Expected 1 event, got %d", len(received))
		}

		if received[0].GetType() != "position_updated" {
			t.Errorf("Expected position_updated event, got %s", received[0].GetType())
		}
	})
}

func TestCommandValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	commandBus := NewCommandBus(logger)

	// Register mock handlers for valid commands to pass
	mockHandler := &MockCommandHandler{}
	commandBus.RegisterHandler(ExecuteTaskCommand{}, mockHandler)
	commandBus.RegisterHandler(SellPositionCommand{}, mockHandler)

	// Тест валидации команд
	tests := []struct {
		name    string
		cmd     TradingCommand
		wantErr bool
	}{
		{
			name: "valid execute task",
			cmd: ExecuteTaskCommand{
				TaskID:    1,
				UserID:    "test_user",
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "invalid execute task - zero ID",
			cmd: ExecuteTaskCommand{
				TaskID:    0,
				UserID:    "test_user",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "valid sell position",
			cmd: SellPositionCommand{
				TokenMint:  "valid_mint",
				Percentage: 50.0,
				UserID:     "test_user",
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "invalid sell position - high percentage",
			cmd: SellPositionCommand{
				TokenMint:  "valid_mint",
				Percentage: 150.0,
				UserID:     "test_user",
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := commandBus.Send(ctx, tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("CommandBus.Send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEventBusPerformance(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := NewEventBus(logger)

	// Создаем несколько подписчиков
	subscribers := make([]*MockEventSubscriber, 5)
	for i := 0; i < 5; i++ {
		subscribers[i] = NewMockEventSubscriber([]string{"task_executed", "position_updated"})
		eventBus.Subscribe(subscribers[i])
	}

	// Отправляем много событий
	start := time.Now()
	eventCount := 100

	for i := 0; i < eventCount; i++ {
		event := TaskExecutedEvent{
			TaskID:      i,
			TaskName:    "test_task",
			TokenMint:   "test_mint",
			TxSignature: "test_tx",
			Success:     true,
			UserID:      "test_user",
			Timestamp:   time.Now(),
		}
		eventBus.Publish(event)
	}

	// Ждем обработки всех событий
	time.Sleep(500 * time.Millisecond)

	duration := time.Since(start)
	t.Logf("Published %d events in %v", eventCount, duration)

	// Проверяем, что все подписчики получили все события
	for i, subscriber := range subscribers {
		received := subscriber.GetReceivedEvents()
		if len(received) != eventCount {
			t.Errorf("Subscriber %d received %d events, expected %d", i, len(received), eventCount)
		}
	}
}
