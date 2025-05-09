// internal/monitor/input.go
package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

// CommandCallback - функция обратного вызова, выполняемая при вводе команды.
type CommandCallback func(command string) error

// InputHandler обрабатывает пользовательский ввод для сессии мониторинга.
type InputHandler struct {
	callbacks map[string]CommandCallback // Map of command -> callback
	logger    *zap.Logger                // Logger
	ctx       context.Context            // Context for cancellation
	cancel    context.CancelFunc         // Cancel function
}

// NewInputHandler создает новый обработчик пользовательского ввода.
func NewInputHandler(parentCtx context.Context, logger *zap.Logger) *InputHandler {
	ctx, cancel := context.WithCancel(parentCtx)
	return &InputHandler{
		callbacks: make(map[string]CommandCallback),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// RegisterCommand регистрирует обработчик для определенной команды.
func (ih *InputHandler) RegisterCommand(command string, callback CommandCallback) {
	ih.callbacks[command] = callback
}

// Start запускает процесс обработки пользовательского ввода.
func (ih *InputHandler) Start() {
	ih.logger.Debug("Starting input handler")

	go func() {
		reader := bufio.NewReader(os.Stdin)

		for {
			select {
			case <-ih.ctx.Done():
				ih.logger.Debug("Input handler stopped")
				return
			default:
				// Read a line non-blocking
				line, err := reader.ReadString('\n')
				if err != nil {
					if ih.ctx.Err() == nil { // Only log if not canceled
						ih.logger.Error("Error reading input", zap.Error(err))
					}
					// Short sleep to prevent CPU spin
					continue
				}

				// Process the command
				command := strings.TrimSpace(line)

				// Empty command is Enter key
				if command == "" {
					if callback, ok := ih.callbacks[""]; ok {
						if err := callback(command); err != nil {
							ih.logger.Error("Error executing command", zap.Error(err))
						}
					}
					continue
				}

				// Check for registered commands
				if callback, ok := ih.callbacks[command]; ok {
					if err := callback(command); err != nil {
						ih.logger.Error("Error executing command", zap.Error(err))
					}
				} else {
					fmt.Println("Unknown command. Press Enter to sell tokens or 'q' to exit.")
				}
			}
		}
	}()
}

// Stop останавливает обработчик пользовательского ввода.
func (ih *InputHandler) Stop() {
	if ih.cancel != nil {
		ih.cancel()
	}
}
