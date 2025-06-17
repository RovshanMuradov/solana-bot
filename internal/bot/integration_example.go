// internal/bot/integration_example.go
package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ExampleTradingHandler демонстрирует реализацию CommandHandler
type ExampleTradingHandler struct {
	logger   *zap.Logger
	eventBus *EventBus
}

func NewExampleTradingHandler(logger *zap.Logger, eventBus *EventBus) *ExampleTradingHandler {
	return &ExampleTradingHandler{
		logger:   logger.Named("trading_handler"),
		eventBus: eventBus,
	}
}

func (h *ExampleTradingHandler) Handle(ctx context.Context, cmd TradingCommand) error {
	switch c := cmd.(type) {
	case ExecuteTaskCommand:
		return h.handleExecuteTask(ctx, c)
	case SellPositionCommand:
		return h.handleSellPosition(ctx, c)
	default:
		return fmt.Errorf("unsupported command type: %s", cmd.GetType())
	}
}

func (h *ExampleTradingHandler) CanHandle(cmd TradingCommand) bool {
	switch cmd.(type) {
	case ExecuteTaskCommand, SellPositionCommand:
		return true
	default:
		return false
	}
}

func (h *ExampleTradingHandler) handleExecuteTask(ctx context.Context, cmd ExecuteTaskCommand) error {
	h.logger.Info("Handling execute task command",
		zap.Int("task_id", cmd.TaskID),
		zap.String("user_id", cmd.UserID))

	// Симуляция выполнения задачи
	time.Sleep(100 * time.Millisecond)

	// Публикуем событие успешного выполнения
	event := TaskExecutedEvent{
		TaskID:      cmd.TaskID,
		TaskName:    fmt.Sprintf("Task_%d", cmd.TaskID),
		TokenMint:   "example_token_mint_" + fmt.Sprintf("%d", cmd.TaskID),
		TxSignature: "example_tx_signature",
		Success:     true,
		UserID:      cmd.UserID,
		Timestamp:   time.Now(),
	}

	h.eventBus.Publish(event)

	// Создаем позицию
	positionEvent := PositionCreatedEvent{
		TaskID:       cmd.TaskID,
		TokenMint:    event.TokenMint,
		TokenSymbol:  "EXAMPLE",
		EntryPrice:   1.0,
		TokenBalance: 1000000,
		AmountSol:    0.1,
		TxSignature:  event.TxSignature,
		UserID:       cmd.UserID,
		Timestamp:    time.Now(),
	}

	h.eventBus.Publish(positionEvent)

	h.logger.Info("Task executed successfully",
		zap.Int("task_id", cmd.TaskID),
		zap.String("tx_signature", event.TxSignature))

	return nil
}

func (h *ExampleTradingHandler) handleSellPosition(ctx context.Context, cmd SellPositionCommand) error {
	h.logger.Info("Handling sell position command",
		zap.String("token_mint", cmd.TokenMint),
		zap.Float64("percentage", cmd.Percentage),
		zap.String("user_id", cmd.UserID))

	// Симуляция продажи
	time.Sleep(150 * time.Millisecond)

	// Публикуем событие продажи
	event := SellCompletedEvent{
		TokenMint:   cmd.TokenMint,
		AmountSold:  cmd.Percentage / 100.0,
		SolReceived: cmd.Percentage / 100.0 * 0.1, // Примерная цена
		TxSignature: "sell_tx_signature",
		Success:     true,
		UserID:      cmd.UserID,
		Timestamp:   time.Now(),
	}

	h.eventBus.Publish(event)

	h.logger.Info("Position sold successfully",
		zap.String("token_mint", cmd.TokenMint),
		zap.Float64("percentage", cmd.Percentage))

	return nil
}

// ExampleUISubscriber демонстрирует подписчика на события для UI
type ExampleUISubscriber struct {
	logger   *zap.Logger
	received []TradingEvent
	mu       sync.Mutex
}

func NewExampleUISubscriber(logger *zap.Logger) *ExampleUISubscriber {
	return &ExampleUISubscriber{
		logger:   logger.Named("ui_subscriber"),
		received: make([]TradingEvent, 0),
	}
}

func (s *ExampleUISubscriber) OnEvent(event TradingEvent) {
	// Добавляем событие в список для тестирования
	s.mu.Lock()
	s.received = append(s.received, event)
	s.mu.Unlock()

	switch e := event.(type) {
	case TaskExecutedEvent:
		s.onTaskExecuted(e)
	case PositionCreatedEvent:
		s.onPositionCreated(e)
	case SellCompletedEvent:
		s.onSellCompleted(e)
	case PositionUpdatedEvent:
		s.onPositionUpdated(e)
	default:
		s.logger.Debug("Unhandled event type", zap.String("type", event.GetType()))
	}
}

func (s *ExampleUISubscriber) GetSubscribedEventTypes() []string {
	return []string{
		"task_executed",
		"position_created",
		"sell_completed",
		"position_updated",
	}
}

func (s *ExampleUISubscriber) onTaskExecuted(event TaskExecutedEvent) {
	if event.Success {
		s.logger.Info("UI: Task execution completed",
			zap.Int("task_id", event.TaskID),
			zap.String("token_mint", event.TokenMint),
			zap.String("tx_signature", event.TxSignature))
	} else {
		s.logger.Error("UI: Task execution failed",
			zap.Int("task_id", event.TaskID),
			zap.String("error", event.Error))
	}
}

func (s *ExampleUISubscriber) onPositionCreated(event PositionCreatedEvent) {
	s.logger.Info("UI: New position created",
		zap.String("token_mint", event.TokenMint),
		zap.String("token_symbol", event.TokenSymbol),
		zap.Float64("entry_price", event.EntryPrice),
		zap.Float64("amount_sol", event.AmountSol))
}

func (s *ExampleUISubscriber) onSellCompleted(event SellCompletedEvent) {
	if event.Success {
		s.logger.Info("UI: Sell completed",
			zap.String("token_mint", event.TokenMint),
			zap.Float64("amount_sold", event.AmountSold),
			zap.Float64("sol_received", event.SolReceived))
	} else {
		s.logger.Error("UI: Sell failed",
			zap.String("token_mint", event.TokenMint),
			zap.String("error", event.Error))
	}
}

func (s *ExampleUISubscriber) onPositionUpdated(event PositionUpdatedEvent) {
	s.logger.Info("UI: Position updated",
		zap.String("token_mint", event.TokenMint),
		zap.Float64("current_price", event.CurrentPrice),
		zap.Float64("pnl_percent", event.PnLPercent))
}

// DemoCommandEventSystem демонстрирует работу системы команд и событий
func DemoCommandEventSystem(logger *zap.Logger) error {
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

	logger.Info("Command/Event system initialized",
		zap.Strings("registered_handlers", commandBus.GetRegisteredHandlers()),
		zap.Int("ui_subscriptions", len(uiSubscriber.GetSubscribedEventTypes())))

	// Демонстрируем выполнение команд
	ctx := context.Background()

	// 1. Выполняем задачу
	executeCmd := ExecuteTaskCommand{
		TaskID:    1,
		UserID:    "demo_user",
		Timestamp: time.Now(),
	}

	logger.Info("Sending execute task command")
	if err := commandBus.Send(ctx, executeCmd); err != nil {
		return fmt.Errorf("failed to execute task: %w", err)
	}

	// Ждем обработки событий
	time.Sleep(200 * time.Millisecond)

	// 2. Продаем позицию
	sellCmd := SellPositionCommand{
		TokenMint:  "example_token_mint_1",
		Percentage: 50.0,
		UserID:     "demo_user",
		Timestamp:  time.Now(),
	}

	logger.Info("Sending sell position command")
	if err := commandBus.Send(ctx, sellCmd); err != nil {
		return fmt.Errorf("failed to sell position: %w", err)
	}

	// Ждем обработки событий
	time.Sleep(200 * time.Millisecond)

	// 3. Симулируем обновление позиции
	positionUpdate := PositionUpdatedEvent{
		TokenMint:    "example_token_mint_1",
		TokenSymbol:  "EXAMPLE",
		CurrentPrice: 1.5,
		EntryPrice:   1.0,
		PnLPercent:   50.0,
		PnLSol:       0.05,
		Amount:       500000,
		UserID:       "demo_user",
		Timestamp:    time.Now(),
	}

	logger.Info("Publishing position update event")
	eventBus.Publish(positionUpdate)

	// Ждем обработки событий
	time.Sleep(100 * time.Millisecond)

	logger.Info("Demo completed successfully")
	return nil
}

// GetReceivedEvents возвращает список полученных событий для тестирования
func (s *ExampleUISubscriber) GetReceivedEvents() []TradingEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TradingEvent(nil), s.received...)
}
