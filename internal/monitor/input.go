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
//
// Функция получает введенную команду в виде строки и возвращает ошибку,
// если выполнение команды завершилось с ошибкой.
//
// Параметры:
//   - command: строка, содержащая введенную пользователем команду
//
// Возвращает:
//   - error: ошибку, если выполнение команды не удалось
type CommandCallback func(command string) error

// InputHandler обрабатывает пользовательский ввод для сессии мониторинга.
//
// Структура позволяет регистрировать обработчики для различных команд и
// асинхронно обрабатывать пользовательский ввод. Поддерживает отмену
// операций через контекст и структурированное логирование.
//
// Поля:
//   - callbacks: отображение команд на их обработчики
//   - logger: логгер для записи информации и ошибок
//   - ctx: контекст для отмены операций
//   - cancel: функция для отмены контекста
type InputHandler struct {
	callbacks map[string]CommandCallback // Map of command -> callback
	logger    *zap.Logger                // Logger
	ctx       context.Context            // Context for cancellation
	cancel    context.CancelFunc         // Cancel function
}

// NewInputHandler создает новый обработчик пользовательского ввода.
//
// Функция инициализирует структуру InputHandler, создает новый контекст
// с функцией отмены и подготавливает карту обработчиков команд.
//
// Параметры:
//   - logger: логгер для записи информации и ошибок
//
// Возвращает:
//   - *InputHandler: новый экземпляр обработчика пользовательского ввода
func NewInputHandler(logger *zap.Logger) *InputHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &InputHandler{
		callbacks: make(map[string]CommandCallback),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// RegisterCommand регистрирует обработчик для определенной команды.
//
// Метод сохраняет функцию обратного вызова в карту обработчиков, связывая
// её с указанной командой. Когда пользователь вводит эту команду, будет
// вызвана соответствующая функция. Если команда уже зарегистрирована,
// предыдущий обработчик будет заменен новым.
//
// Параметры:
//   - command: строка-команда, которую должен ввести пользователь
//   - callback: функция, которая будет вызвана при вводе команды
func (ih *InputHandler) RegisterCommand(command string, callback CommandCallback) {
	ih.callbacks[command] = callback
}

// Start запускает процесс обработки пользовательского ввода.
//
// Метод запускает отдельную горутину, которая читает ввод пользователя из
// стандартного потока ввода (os.Stdin) и обрабатывает введенные команды.
// Пустая строка (нажатие Enter) обрабатывается как специальная команда.
// Обработка продолжается до тех пор, пока контекст не будет отменен
// через метод Stop.
//
// Особенности работы:
//   - Для пустой строки вызывается обработчик с пустым ключом, если он зарегистрирован
//   - Для неизвестных команд выводится сообщение с подсказкой
//   - Ошибки чтения ввода и выполнения команд логируются, но не прерывают обработку
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
//
// Метод отменяет контекст, что приводит к завершению горутины,
// обрабатывающей пользовательский ввод. Это безопасный способ
// остановки обработчика, гарантирующий корректное завершение
// всех внутренних процессов.
//
// Метод безопасен для многократного вызова и при вызове на
// уже остановленном обработчике не производит никаких действий.
func (ih *InputHandler) Stop() {
	if ih.cancel != nil {
		ih.cancel()
	}
}
