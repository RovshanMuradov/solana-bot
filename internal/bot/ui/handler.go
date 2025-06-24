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
	ExitRequested                  // Запрос на выход без продажи (q/exit)
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
	h.logger.Info("📊 Monitoring started - Press Enter to sell tokens or 'q' to exit")

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
					h.logger.Warn("⚠️ Unknown command received", zap.String("command", command))
					h.logger.Info("💡 Use Enter to sell tokens or 'q' to exit")
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

// shortenAddress возвращает усечённый вид адреса вида "6QwKg…JVuJpump"
func shortenAddress(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:6] + "…" + addr[len(addr)-6:]
}

// Render отправляет информацию о мониторинге через структурированные логи вместо прямого вывода в консоль
func Render(update monitor.PriceUpdate, pnl model.PnLResult, tokenMint string) {
	// Получаем логгер (в идеале он должен передаваться как параметр)
	// Для совместимости создаем базовый логгер
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Определяем тренд цены
	trendIcon := "📈"
	if update.Percent < 0 {
		trendIcon = "📉"
	} else if update.Percent == 0 {
		trendIcon = "📊"
	}

	// Определяем статус PnL
	pnlIcon := "💰"
	if pnl.NetPnL < 0 {
		pnlIcon = "📉"
	} else if pnl.NetPnL > 0 {
		pnlIcon = "📈"
	}

	// Отправляем структурированные логи вместо прямого вывода в консоль
	logger.Info("💹 Price update",
		zap.String("token", shortenAddress(tokenMint)),
		zap.Float64("current_price", update.Current),
		zap.Float64("initial_price", update.Initial),
		zap.Float64("change_percent", update.Percent),
		zap.Float64("tokens_owned", update.Tokens),
		zap.String("trend", trendIcon),
	)

	logger.Info(fmt.Sprintf("%s PnL update", pnlIcon),
		zap.Float64("net_pnl", pnl.NetPnL),
		zap.Float64("pnl_percentage", pnl.PnLPercentage),
		zap.Float64("sell_estimate", pnl.SellEstimate),
		zap.Float64("initial_investment", pnl.InitialInvestment),
		zap.String("token", shortenAddress(tokenMint)),
	)

	logger.Info("⌨️ Commands available - Press Enter to sell, 'q' to exit")
}
