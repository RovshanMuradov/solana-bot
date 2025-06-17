// internal/bot/commands_test.go
package bot

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// MockCommandHandler для тестирования
type MockCommandHandler struct {
	handled []TradingCommand
	errors  map[string]error
}

func NewMockCommandHandler() *MockCommandHandler {
	return &MockCommandHandler{
		handled: make([]TradingCommand, 0),
		errors:  make(map[string]error),
	}
}

func (h *MockCommandHandler) Handle(ctx context.Context, cmd TradingCommand) error {
	h.handled = append(h.handled, cmd)
	if err, exists := h.errors[cmd.GetType()]; exists {
		return err
	}
	return nil
}

func (h *MockCommandHandler) CanHandle(cmd TradingCommand) bool {
	return true
}

func (h *MockCommandHandler) SetError(cmdType string, err error) {
	h.errors[cmdType] = err
}

func (h *MockCommandHandler) GetHandledCommands() []TradingCommand {
	return h.handled
}

func TestExecuteTaskCommand_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ExecuteTaskCommand
		wantErr bool
	}{
		{
			name: "valid command",
			cmd: ExecuteTaskCommand{
				TaskID:    1,
				UserID:    "test_user",
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "invalid task_id",
			cmd: ExecuteTaskCommand{
				TaskID:    0,
				UserID:    "test_user",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "empty user_id",
			cmd: ExecuteTaskCommand{
				TaskID:    1,
				UserID:    "",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecuteTaskCommand.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSellPositionCommand_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     SellPositionCommand
		wantErr bool
	}{
		{
			name: "valid command",
			cmd: SellPositionCommand{
				TokenMint:  "test_token_mint",
				Percentage: 50.0,
				UserID:     "test_user",
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "empty token_mint",
			cmd: SellPositionCommand{
				TokenMint:  "",
				Percentage: 50.0,
				UserID:     "test_user",
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
		{
			name: "invalid percentage (too low)",
			cmd: SellPositionCommand{
				TokenMint:  "test_token_mint",
				Percentage: 0,
				UserID:     "test_user",
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
		{
			name: "invalid percentage (too high)",
			cmd: SellPositionCommand{
				TokenMint:  "test_token_mint",
				Percentage: 150.0,
				UserID:     "test_user",
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SellPositionCommand.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommandBus_RegisterHandler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewCommandBus(logger)
	handler := NewMockCommandHandler()

	// Регистрируем обработчик
	bus.RegisterHandler(ExecuteTaskCommand{}, handler)

	handlers := bus.GetRegisteredHandlers()
	if len(handlers) != 1 {
		t.Errorf("Expected 1 registered handler, got %d", len(handlers))
	}

	if handlers[0] != "execute_task" {
		t.Errorf("Expected handler for 'execute_task', got '%s'", handlers[0])
	}
}

func TestCommandBus_Send_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewCommandBus(logger)
	handler := NewMockCommandHandler()

	// Регистрируем обработчик
	bus.RegisterHandler(ExecuteTaskCommand{}, handler)

	// Создаем валидную команду
	cmd := ExecuteTaskCommand{
		TaskID:    1,
		UserID:    "test_user",
		Timestamp: time.Now(),
	}

	// Отправляем команду
	ctx := context.Background()
	err := bus.Send(ctx, cmd)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Проверяем, что команда была обработана
	handled := handler.GetHandledCommands()
	if len(handled) != 1 {
		t.Errorf("Expected 1 handled command, got %d", len(handled))
	}

	if handled[0].GetType() != "execute_task" {
		t.Errorf("Expected 'execute_task' command, got '%s'", handled[0].GetType())
	}
}

func TestCommandBus_Send_ValidationError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewCommandBus(logger)
	handler := NewMockCommandHandler()

	// Регистрируем обработчик
	bus.RegisterHandler(ExecuteTaskCommand{}, handler)

	// Создаем невалидную команду
	cmd := ExecuteTaskCommand{
		TaskID: 0, // невалидный ID
		UserID: "test_user",
	}

	// Отправляем команду
	ctx := context.Background()
	err := bus.Send(ctx, cmd)

	if err == nil {
		t.Error("Expected validation error, got nil")
	}

	// Проверяем, что команда НЕ была обработана
	handled := handler.GetHandledCommands()
	if len(handled) != 0 {
		t.Errorf("Expected 0 handled commands, got %d", len(handled))
	}
}

func TestCommandBus_Send_NoHandler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewCommandBus(logger)

	// НЕ регистрируем обработчик

	// Создаем валидную команду
	cmd := ExecuteTaskCommand{
		TaskID:    1,
		UserID:    "test_user",
		Timestamp: time.Now(),
	}

	// Отправляем команду
	ctx := context.Background()
	err := bus.Send(ctx, cmd)

	if err == nil {
		t.Error("Expected 'no handler' error, got nil")
	}
}

func TestCommandBus_Send_HandlerError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	bus := NewCommandBus(logger)
	handler := NewMockCommandHandler()

	// Настраиваем обработчик для возврата ошибки
	handler.SetError("execute_task", fmt.Errorf("handler error"))

	// Регистрируем обработчик
	bus.RegisterHandler(ExecuteTaskCommand{}, handler)

	// Создаем валидную команду
	cmd := ExecuteTaskCommand{
		TaskID:    1,
		UserID:    "test_user",
		Timestamp: time.Now(),
	}

	// Отправляем команду
	ctx := context.Background()
	err := bus.Send(ctx, cmd)

	if err == nil {
		t.Error("Expected handler error, got nil")
	}

	// Проверяем, что команда была обработана (несмотря на ошибку)
	handled := handler.GetHandledCommands()
	if len(handled) != 1 {
		t.Errorf("Expected 1 handled command, got %d", len(handled))
	}
}
