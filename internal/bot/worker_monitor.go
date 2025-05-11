// internal/bot/worker_monitor.go
package bot

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/bot/ui"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

// SellFunc представляет функцию для продажи токенов
type SellFunc func(ctx context.Context, percent float64) error

// MonitorWorker представляет рабочий процесс мониторинга
type MonitorWorker struct {
	ctx      context.Context
	logger   *zap.Logger
	task     *task.Task
	dex      dex.DEX
	session  *monitor.MonitoringSession
	uiHandle *ui.Handler
	sellFn   SellFunc
}

// NewMonitorWorker создает новый экземпляр рабочего процесса мониторинга
func NewMonitorWorker(
	ctx context.Context,
	t *task.Task,
	dexAdapter dex.DEX,
	logger *zap.Logger,
	tokenBalance uint64,
	initialPrice float64,
	monitorInterval time.Duration,
	sellFn SellFunc,
) *MonitorWorker {
	return &MonitorWorker{
		ctx:    ctx,
		logger: logger.Named("monitor_worker"),
		task:   t,
		dex:    dexAdapter,
		sellFn: sellFn,
	}
}

// Start запускает рабочий процесс мониторинга
func (mw *MonitorWorker) Start() error {
	// Создаем конфигурацию сессии мониторинга
	monitorConfig := &monitor.SessionConfig{
		Task:            mw.task,
		TokenBalance:    0, // Баланс будет получен автоматически в сессии
		InitialPrice:    0,
		DEX:             mw.dex,
		Logger:          mw.logger.Named("session"),
		MonitorInterval: 1 * time.Second, // Используем стандартный интервал
	}

	// Создаем пользовательский интерфейс
	mw.uiHandle = ui.NewHandler(mw.ctx, mw.logger)

	// Создаем сессию мониторинга
	mw.session = monitor.NewMonitoringSession(mw.ctx, monitorConfig)

	// Создаем группу ошибок для отслеживания всех горутин
	g, gCtx := errgroup.WithContext(mw.ctx)

	// Запускаем сессию мониторинга
	if err := mw.session.Start(); err != nil {
		return fmt.Errorf("failed to start monitoring session: %w", err)
	}

	// Запускаем обработчик пользовательского ввода
	mw.uiHandle.Start()

	// Горутина для обработки событий пользовательского интерфейса
	g.Go(func() error {
		return mw.handleUIEvents(gCtx)
	})

	// Горутина для обработки обновлений цены
	g.Go(func() error {
		return mw.handlePriceUpdates(gCtx)
	})

	// Горутина для обработки ошибок сессии мониторинга
	g.Go(func() error {
		return mw.handleSessionErrors(gCtx)
	})

	// Ожидаем завершения всех горутин
	if err := g.Wait(); err != nil {
		mw.logger.Error("Monitor worker failed", zap.Error(err))
		return err
	}

	return nil
}

// Stop останавливает рабочий процесс мониторинга
func (mw *MonitorWorker) Stop() {
	if mw.uiHandle != nil {
		mw.uiHandle.Stop()
	}
	if mw.session != nil {
		mw.session.Stop()
	}
}

// handleUIEvents processes UI events and initiates sale or exit
func (mw *MonitorWorker) handleUIEvents(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-mw.uiHandle.Events():
			if !ok {
				return nil // channel closed
			}

			switch event.Type {
			case ui.SellRequested:
				mw.logger.Info("Sell requested by user")
				// Immediately stop UI updates and price monitoring
				mw.Stop()

				// Launch token sale independently of monitoring context
				go func() {
					sellCtx, cancel := context.WithCancel(context.Background())
					defer cancel()

					fmt.Println("\nSelling tokens...")

					if err := mw.sellFn(sellCtx, 100.0); err != nil { // TODO: percent hard coded
						mw.logger.Error("Failed to sell tokens", zap.Error(err))
						fmt.Printf("Error selling tokens: %v\n", err)
					} else {
						fmt.Println("Tokens sold successfully!")
					}
				}()
				return nil

			case ui.ExitRequested:
				mw.logger.Info("Exit requested by user")
				fmt.Println("\nExiting monitor mode without selling tokens.")
				mw.Stop()
				return nil
			}
		}
	}
}

// handlePriceUpdates обрабатывает обновления цены от сессии мониторинга
func (mw *MonitorWorker) handlePriceUpdates(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update, ok := <-mw.session.PriceUpdates():
			if !ok {
				return nil // Канал закрыт
			}

			// Расчет PnL
			pnlData, err := mw.calculatePnL(ctx, update)
			if err != nil {
				mw.logger.Error("Failed to calculate PnL", zap.Error(err))
				continue
			}

			// Отображение информации через UI
			ui.Render(update, *pnlData, mw.task.TokenMint)
		}
	}
}

// handleSessionErrors обрабатывает ошибки от сессии мониторинга
func (mw *MonitorWorker) handleSessionErrors(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-mw.session.Err():
			if !ok {
				return nil // Канал закрыт
			}
			mw.logger.Error("Session error", zap.Error(err))
			return err // Возвращаем ошибку, чтобы завершить группу
		}
	}
}

// calculatePnL рассчитывает PnL на основе обновления цены
func (mw *MonitorWorker) calculatePnL(ctx context.Context, update monitor.PriceUpdate) (*model.PnLResult, error) {
	calculator, err := monitor.GetCalculator(mw.dex, mw.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to get calculator for DEX: %w", err)
	}

	pnlData, err := calculator.CalculatePnL(ctx, update.Tokens, mw.task.AmountSol)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate PnL: %w", err)
	}

	return pnlData, nil
}
