// internal/bot/ui/handler.go
package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"go.uber.org/zap"
)

// EventType представляет тип события от UI
type EventType int

const (
	SellRequested EventType = iota // Запрос на продажу токенов (пустая строка)
	ExitRequested                 // Запрос на выход без продажи (q/exit)
)

// Event представляет событие от пользовательского интерфейса
type Event struct {
	Type EventType // Тип события
	Data string    // Дополнительные данные события (если нужны)
}

// Handler обрабатывает пользовательский ввод и отображение
type Handler struct {
	logger    *zap.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	eventChan chan Event
}

// NewHandler создает новый обработчик UI
func NewHandler(parentCtx context.Context, logger *zap.Logger) *Handler {
	ctx, cancel := context.WithCancel(parentCtx)
	return &Handler{
		logger:    logger.Named("ui"),
		ctx:       ctx,
		cancel:    cancel,
		eventChan: make(chan Event),
	}
}

// Start запускает обработку пользовательского ввода
func (h *Handler) Start() {
	h.logger.Debug("Starting UI handler")
	fmt.Println("\nMonitoring started. Press Enter to sell tokens or 'q' to exit.")

	go func() {
		reader := bufio.NewReader(os.Stdin)

		for {
			select {
			case <-h.ctx.Done():
				h.logger.Debug("UI handler stopped due to context cancellation")
				return
			default:
				// Read a line non-blocking
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						// EOF means stdin is closed or detached
						h.logger.Warn("Stdin closed or detached, exiting UI handler")
						h.publishEvent(ExitRequested, "")
						return
					}

					if h.ctx.Err() == nil { // Only log if not canceled
						h.logger.Error("Error reading input", zap.Error(err))
					}
					continue
				}

				// Process the command
				command := strings.TrimSpace(line)
				
				// Обрабатываем команды
				switch command {
				case "":
					// Пустая строка - запрос на продажу
					h.publishEvent(SellRequested, "")
				case "q", "exit":
					// Запрос на выход
					h.publishEvent(ExitRequested, "")
				default:
					fmt.Println("Unknown command. Press Enter to sell tokens or 'q' to exit.")
				}
			}
		}
	}()
}

// Stop останавливает обработчик
func (h *Handler) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	close(h.eventChan)
}

// Events возвращает канал событий
func (h *Handler) Events() <-chan Event {
	return h.eventChan
}

// publishEvent отправляет событие в канал
func (h *Handler) publishEvent(eventType EventType, data string) {
	select {
	case <-h.ctx.Done():
		return
	case h.eventChan <- Event{Type: eventType, Data: data}:
		h.logger.Debug("Published UI event", zap.Int("type", int(eventType)))
	}
}

// Render отображает информацию о мониторинге в консоли
func Render(update monitor.PriceUpdate, pnl model.PnLResult, tokenMint string) {
	// Форматирование процента изменения цены
	changeStr := fmt.Sprintf("%.2f%%", update.Percent)
	if update.Percent > 0 {
		changeStr = "\033[32m+" + changeStr + "\033[0m" // Зеленый для роста
	} else if update.Percent < 0 {
		changeStr = "\033[31m" + changeStr + "\033[0m" // Красный для падения
	}

	// Форматирование PnL
	pnlStr := fmt.Sprintf("%.8f SOL (%.2f%%)", pnl.NetPnL, pnl.PnLPercentage)
	if pnl.NetPnL > 0 {
		pnlStr = "\033[32m+" + pnlStr + "\033[0m" // Зеленый для прибыли
	} else if pnl.NetPnL < 0 {
		pnlStr = "\033[31m" + pnlStr + "\033[0m" // Красный для убытка
	}

	// Вывод информации в консоль
	fmt.Println("\n╔════════════════ TOKEN MONITOR ════════════════╗")
	fmt.Printf("║ Token: %-38s ║\n", shortenAddress(tokenMint))
	fmt.Println("╟───────────────────────────────────────────────╢")
	fmt.Printf("║ Current Price:       %-14.8f SOL ║\n", update.Current)
	fmt.Printf("║ Initial Price:       %-14.8f SOL ║\n", update.Initial)
	fmt.Printf("║ Price Change:        %-25s ║\n", changeStr)
	fmt.Printf("║ Tokens Owned:        %-14.6f      ║\n", update.Tokens)
	fmt.Println("╟───────────────────────────────────────────────╢")
	fmt.Printf("║ Sold (Estimate):     %-14.8f SOL ║\n", pnl.SellEstimate)
	fmt.Printf("║ Invested:            %-14.8f SOL ║\n", pnl.InitialInvestment)
	fmt.Printf("║ P&L:                 %-25s ║\n", pnlStr)
	fmt.Println("╚═══════════════════════════════════════════════╝")
	fmt.Println("Press Enter to sell tokens, 'q' to exit without selling")
}

// shortenAddress сокращает длинный адрес токена для лучшего отображения
func shortenAddress(address string) string {
	if len(address) <= 20 {
		return address
	}
	return address[:8] + "..." + address[len(address)-8:]
}