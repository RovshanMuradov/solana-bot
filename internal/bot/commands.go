// internal/bot/commands.go
package bot

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TradingCommand представляет команду для выполнения торговых операций
type TradingCommand interface {
	GetType() string
	GetUserID() string
	Validate() error
}

// ExecuteTaskCommand команда для выполнения торговой задачи
type ExecuteTaskCommand struct {
	TaskID    int       `json:"task_id"`
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (c ExecuteTaskCommand) GetType() string {
	return "execute_task"
}

func (c ExecuteTaskCommand) GetUserID() string {
	return c.UserID
}

func (c ExecuteTaskCommand) Validate() error {
	if c.TaskID <= 0 {
		return fmt.Errorf("task_id must be positive, got: %d", c.TaskID)
	}
	if c.UserID == "" {
		return fmt.Errorf("user_id cannot be empty")
	}
	return nil
}

// SellPositionCommand команда для продажи позиции
type SellPositionCommand struct {
	TokenMint  string    `json:"token_mint"`
	Percentage float64   `json:"percentage"`
	UserID     string    `json:"user_id"`
	Timestamp  time.Time `json:"timestamp"`
}

func (c SellPositionCommand) GetType() string {
	return "sell_position"
}

func (c SellPositionCommand) GetUserID() string {
	return c.UserID
}

func (c SellPositionCommand) Validate() error {
	if c.TokenMint == "" {
		return fmt.Errorf("token_mint cannot be empty")
	}
	if c.Percentage <= 0 || c.Percentage > 100 {
		return fmt.Errorf("percentage must be between 0 and 100, got: %f", c.Percentage)
	}
	if c.UserID == "" {
		return fmt.Errorf("user_id cannot be empty")
	}
	return nil
}

// RefreshDataCommand команда для обновления данных
type RefreshDataCommand struct {
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (c RefreshDataCommand) GetType() string {
	return "refresh_data"
}

func (c RefreshDataCommand) GetUserID() string {
	return c.UserID
}

func (c RefreshDataCommand) Validate() error {
	if c.UserID == "" {
		return fmt.Errorf("user_id cannot be empty")
	}
	return nil
}

// CommandHandler интерфейс для обработчиков команд
type CommandHandler interface {
	Handle(ctx context.Context, cmd TradingCommand) error
	CanHandle(cmd TradingCommand) bool
}

// CommandBus шина для обработки команд
type CommandBus struct {
	handlers map[reflect.Type]CommandHandler
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewCommandBus создает новую шину команд
func NewCommandBus(logger *zap.Logger) *CommandBus {
	return &CommandBus{
		handlers: make(map[reflect.Type]CommandHandler),
		logger:   logger.Named("command_bus"),
	}
}

// RegisterHandler регистрирует обработчик для типа команды
func (bus *CommandBus) RegisterHandler(cmdType TradingCommand, handler CommandHandler) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	cmdReflectType := reflect.TypeOf(cmdType)
	bus.handlers[cmdReflectType] = handler

	bus.logger.Info("Command handler registered",
		zap.String("command_type", cmdType.GetType()),
		zap.String("handler", reflect.TypeOf(handler).String()))
}

// Send отправляет команду на выполнение
func (bus *CommandBus) Send(ctx context.Context, cmd TradingCommand) error {
	// Валидация команды
	if err := cmd.Validate(); err != nil {
		bus.logger.Error("Command validation failed",
			zap.String("command_type", cmd.GetType()),
			zap.String("user_id", cmd.GetUserID()),
			zap.Error(err))
		return fmt.Errorf("command validation failed: %w", err)
	}

	bus.mu.RLock()
	cmdType := reflect.TypeOf(cmd)
	handler, exists := bus.handlers[cmdType]
	bus.mu.RUnlock()

	if !exists {
		err := fmt.Errorf("no handler registered for command type: %s", cmd.GetType())
		bus.logger.Error("No handler for command",
			zap.String("command_type", cmd.GetType()),
			zap.String("user_id", cmd.GetUserID()))
		return err
	}

	bus.logger.Info("Executing command",
		zap.String("command_type", cmd.GetType()),
		zap.String("user_id", cmd.GetUserID()))

	// Выполнение команды
	if err := handler.Handle(ctx, cmd); err != nil {
		bus.logger.Error("Command execution failed",
			zap.String("command_type", cmd.GetType()),
			zap.String("user_id", cmd.GetUserID()),
			zap.Error(err))
		return fmt.Errorf("command execution failed: %w", err)
	}

	bus.logger.Info("Command executed successfully",
		zap.String("command_type", cmd.GetType()),
		zap.String("user_id", cmd.GetUserID()))

	return nil
}

// GetRegisteredHandlers возвращает список зарегистрированных обработчиков
func (bus *CommandBus) GetRegisteredHandlers() []string {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	handlers := make([]string, 0, len(bus.handlers))
	for cmdType := range bus.handlers {
		// Создаем экземпляр для получения типа команды
		var cmd TradingCommand
		if cmdType.Kind() == reflect.Ptr {
			cmd = reflect.New(cmdType.Elem()).Interface().(TradingCommand)
		} else {
			cmd = reflect.New(cmdType).Elem().Interface().(TradingCommand)
		}
		handlers = append(handlers, cmd.GetType())
	}
	return handlers
}
