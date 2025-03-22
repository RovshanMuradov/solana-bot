package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

// CommandCallback is a function called when a command is entered
type CommandCallback func(command string) error

// InputHandler handles user input for the monitoring session
type InputHandler struct {
	callbacks map[string]CommandCallback // Map of command -> callback
	logger    *zap.Logger                // Logger
	ctx       context.Context            // Context for cancellation
	cancel    context.CancelFunc         // Cancel function
}

// NewInputHandler creates a new input handler
func NewInputHandler(logger *zap.Logger) *InputHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &InputHandler{
		callbacks: make(map[string]CommandCallback),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// RegisterCommand registers a callback for a specific command
func (ih *InputHandler) RegisterCommand(command string, callback CommandCallback) {
	ih.callbacks[command] = callback
}

// Start begins the input handling process
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

// Stop stops the input handler
func (ih *InputHandler) Stop() {
	if ih.cancel != nil {
		ih.cancel()
	}
}
