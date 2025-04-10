package monitor

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// SessionConfig contains configuration for a monitoring session
type SessionConfig struct {
	TokenMint       string        // Token mint address
	TokenAmount     float64       // Human-readable amount of tokens purchased
	TokenBalance    uint64        // Raw token balance in smallest units
	InitialAmount   float64       // Initial SOL amount spent
	InitialPrice    float64       // Initial token price
	MonitorInterval time.Duration // Interval for price updates
	DEX             dex.DEX       // DEX interface
	Logger          *zap.Logger   // Logger

	// Transaction parameters from the original task
	SlippagePercent float64 // Slippage percentage for transactions
	PriorityFee     string  // Priority fee for transactions
	ComputeUnits    uint32  // Compute units for transactions
}

// MonitoringSession представляет сессию мониторинга токенов для операций на DEX.
//
// Структура объединяет мониторинг цены и обработку пользовательского ввода для
// отслеживания стоимости токенов в реальном времени и выполнения операций продажи.
// Использует контекст для координации работы компонентов и WaitGroup для обеспечения
// корректного завершения всех операций.
//
// Поля:
//   - config: конфигурация сессии мониторинга
//   - logger: логгер для записи информации и ошибок
//   - ctx: контекст для отмены операций
//   - cancel: функция для отмены контекста
//   - priceMonitor: компонент для мониторинга цены токена
//   - inputHandler: компонент для обработки пользовательского ввода
//   - wg: WaitGroup для синхронизации горутин
type MonitoringSession struct {
	config       *SessionConfig
	priceMonitor *PriceMonitor
	inputHandler *InputHandler
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *zap.Logger
}

// NewMonitoringSession создает новую сессию мониторинга.
//
// Функция инициализирует структуру MonitoringSession, создает новый контекст
// с функцией отмены и настраивает все необходимые параметры для отслеживания
// цены токена и обработки пользовательского ввода.
//
// Параметры:
//   - config: конфигурация сессии мониторинга, содержащая все необходимые параметры
//
// Возвращает:
//   - *MonitoringSession: новый экземпляр сессии мониторинга
func NewMonitoringSession(config *SessionConfig) *MonitoringSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &MonitoringSession{
		config: config,
		logger: config.Logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start запускает сессию мониторинга.
//
// Метод инициализирует и запускает компоненты мониторинга цены и обработки
// пользовательского ввода. Регистрирует необходимые обработчики команд
// и запускает мониторинг цены в отдельной горутине. Отображает пользователю
// информацию о запуске мониторинга и доступных командах.
//
// Возвращает:
//   - error: ошибку, если не удалось запустить сессию мониторинга
func (ms *MonitoringSession) Start() error {
	// Create a price monitor
	ms.priceMonitor = NewPriceMonitor(
		ms.config.DEX,
		ms.config.TokenMint,
		ms.config.InitialPrice,
		ms.config.TokenAmount,
		ms.config.InitialAmount,
		ms.config.MonitorInterval,
		ms.logger.Named("price"),
		ms.onPriceUpdate,
	)

	// Create an input handler
	ms.inputHandler = NewInputHandler(ms.logger.Named("input"))

	// Register commands
	ms.inputHandler.RegisterCommand("", ms.onEnterPressed) // Empty command (Enter key)
	ms.inputHandler.RegisterCommand("q", ms.onExitCommand)
	ms.inputHandler.RegisterCommand("exit", ms.onExitCommand)

	// Start the components
	ms.wg.Add(1)
	go func() {
		defer ms.wg.Done()
		ms.priceMonitor.Start()
	}()

	ms.inputHandler.Start()

	// Показать простое сообщение о начале мониторинга
	fmt.Println("\nMonitoring started. Press Enter to sell tokens or 'q' to exit.")
	return nil
}

// Wait ожидает завершения сессии мониторинга.
//
// Метод блокирует выполнение до тех пор, пока все горутины, связанные с сессией
// мониторинга, не завершат свою работу. Используется для обеспечения корректного
// завершения всех операций перед выходом из программы.
//
// Возвращает:
//   - error: ошибку, если возникли проблемы при ожидании завершения
func (ms *MonitoringSession) Wait() error {
	ms.wg.Wait()
	return nil
}

// Stop останавливает сессию мониторинга.
//
// Метод останавливает все компоненты сессии мониторинга в правильном порядке:
// сначала обработчик ввода, затем монитор цены, и в конце отменяет контекст.
// Это обеспечивает корректное завершение всех внутренних процессов.
//
// Метод безопасен для многократного вызова и при вызове на уже остановленной
// сессии не производит никаких действий.
func (ms *MonitoringSession) Stop() {
	// Stop the input handler
	if ms.inputHandler != nil {
		ms.inputHandler.Stop()
	}

	// Stop the price monitor
	if ms.priceMonitor != nil {
		ms.priceMonitor.Stop()
	}

	// Cancel the context
	if ms.cancel != nil {
		ms.cancel()
	}
}

// UpdateWithDiscretePnL обновляет сессию мониторинга с учетом дискретного расчета PnL.
//
// Метод изменяет функцию обратного вызова для обработки обновлений цены,
// чтобы использовать дискретный (более точный) метод расчета прибыли и убытков.
// Этот подход учитывает особенности работы DEX и предоставляет более точную
// оценку текущего состояния инвестиции.
//
// Возвращает:
//   - error: ошибку, если не удалось обновить сессию мониторинга
func (ms *MonitoringSession) UpdateWithDiscretePnL() error {
	// Меняем callback для обработки обновлений цены
	ms.priceMonitor.SetCallback(func(currentPrice, initialPrice, percentChange, tokenAmount float64) {
		// Используем измененную функцию onPriceUpdate
		ms.onPriceUpdate(currentPrice, initialPrice, percentChange, tokenAmount)
	})

	return nil
}

// onPriceUpdate вызывается при обновлении цены токена.
//
// Метод получает актуальную информацию о цене токена и его балансе,
// вычисляет прибыль/убытки (PnL) и выводит эту информацию в консоль.
// Метод поддерживает два режима расчета PnL:
// 1. Дискретный (более точный) - учитывает особенности DEX
// 2. Стандартный - использует прямой расчет на основе текущей цены
//
// Метод также форматирует вывод с цветовым оформлением для лучшего
// визуального представления изменений цены и PnL.
//
// Параметры:
//   - currentPrice: текущая цена токена в SOL
//   - initialPrice: начальная цена токена в SOL
//   - percentChange: процентное изменение цены
//   - _: количество токенов (не используется, так как получается из актуального баланса)
//
// TODO: probably need rewrite
func (ms *MonitoringSession) onPriceUpdate(currentPrice, initialPrice, percentChange, _ float64) {
	// Получаем актуальный баланс токенов через RPC
	balanceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	actualTokenBalance, err := ms.config.DEX.GetTokenBalance(balanceCtx, ms.config.TokenMint)

	// TODO: work with balance
	// Рассчитываем актуальное человекочитаемое количество токенов
	tokenAmount := ms.config.TokenAmount // Используем старое значение, если не удалось получить новое
	if err == nil && actualTokenBalance > 0 {
		// Конвертируем актуальный баланс в человекочитаемый формат
		tokenDecimals := 6 // По умолчанию 6 знаков (стандарт для многих токенов)
		tokenAmount = float64(actualTokenBalance) / math.Pow10(int(tokenDecimals))

		ms.logger.Debug("Updated token balance",
			zap.Uint64("raw_balance", actualTokenBalance),
			zap.Float64("human_amount", tokenAmount))
	} else if err != nil {
		ms.logger.Debug("Could not fetch actual token balance", zap.Error(err))
	}

	// Стандартный расчет PnL с актуальным балансом
	currentValue := currentPrice * tokenAmount
	profit := currentValue - ms.config.InitialAmount
	profitPercent := 0.0
	if ms.config.InitialAmount > 0 {
		profitPercent = (profit / ms.config.InitialAmount) * 100
	}

	// Пытаемся вычислить более точный PnL для Pump.fun
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	discretePnL, err := ms.config.DEX.CalculateDiscretePnL(ctx, tokenAmount, ms.config.InitialAmount)

	// Компактный однострочный формат вывода
	var pnlText string

	if err == nil && discretePnL != nil {
		// Цветовое оформление для изменения цены и PnL
		priceChangeColor := "\033[0m" // Нейтральный
		if percentChange > 0 {
			priceChangeColor = "\033[32m" // Зеленый
		} else if percentChange < 0 {
			priceChangeColor = "\033[31m" // Красный
		}

		pnlColor := "\033[0m" // Нейтральный
		if discretePnL.NetPnL > 0 {
			pnlColor = "\033[32m" // Зеленый
		} else if discretePnL.NetPnL < 0 {
			pnlColor = "\033[31m" // Красный
		}

		// Форматирование в одну строку
		pnlText = fmt.Sprintf("\n=== %s Discrete PnL ===\n", ms.config.DEX.GetName()) +
			fmt.Sprintf("Entry Price: %.9f SOL | Current Price: %.9f SOL | Change: %s%.2f%%\033[0m\n",
				initialPrice, discretePnL.CurrentPrice, priceChangeColor, percentChange) +
			fmt.Sprintf("Tokens: %.6f | Theoretical Value: %.6f SOL | Sell Estimate: %.6f SOL\n",
				tokenAmount, discretePnL.TheoreticalValue, discretePnL.SellEstimate) +
			fmt.Sprintf("Initial Investment: %.6f SOL | Net PnL: %s%.6f SOL (%.2f%%)\033[0m\n",
				discretePnL.InitialInvestment, pnlColor, discretePnL.NetPnL, discretePnL.PnLPercentage) +
			fmt.Sprintf("===========================\n")
	} else {
		// Стандартный расчет, если дискретный недоступен
		priceChangeColor := "\033[0m" // Нейтральный
		if percentChange > 0 {
			priceChangeColor = "\033[32m" // Зеленый
		} else if percentChange < 0 {
			priceChangeColor = "\033[31m" // Красный
		}

		pnlColor := "\033[0m" // Нейтральный
		if profit > 0 {
			pnlColor = "\033[32m" // Зеленый
		} else if profit < 0 {
			pnlColor = "\033[31m" // Красный
		}

		pnlText = fmt.Sprintf("\n=== %s PnL ===\n", ms.config.DEX.GetName()) +
			fmt.Sprintf("Entry Price: %.9f SOL | Current Price: %.9f SOL | Change: %s%.2f%%\033[0m\n",
				initialPrice, currentPrice, priceChangeColor, percentChange) +
			fmt.Sprintf("Tokens: %.6f | Value: %.6f SOL\n", tokenAmount, currentValue) +
			fmt.Sprintf("Initial Investment: %.6f SOL | Net PnL: %s%.6f SOL (%.2f%%)\033[0m\n",
				ms.config.InitialAmount, pnlColor, profit, profitPercent) +
			fmt.Sprintf("===========================\n")
	}

	// Вывод информации и инструкции
	fmt.Println(pnlText)
	fmt.Println("Press Enter to sell tokens or 'q' to exit.")
}

// onEnterPressed вызывается при нажатии клавиши Enter.
//
// Метод инициирует процесс продажи токенов, останавливая сессию мониторинга
// и выполняя операцию продажи через интерфейс DEX. По умолчанию продается
// 99% имеющихся токенов с учетом настроенного проскальзывания и приоритета.
//
// Параметры:
//   - _: строка команды (не используется, всегда пустая строка)
//
// Возвращает:
//   - error: ошибку, если не удалось выполнить продажу токенов
func (ms *MonitoringSession) onEnterPressed(_ string) error {
	fmt.Println("\nSelling tokens...")

	// Останавливаем сессию мониторинга
	ms.Stop()

	// Процент токенов для продажи (99%)
	percentToSell := 99.0

	ms.logger.Info("Executing sell operation",
		zap.String("token_mint", ms.config.TokenMint),
		zap.Float64("percent_to_sell", percentToSell),
		zap.Float64("slippage_percent", ms.config.SlippagePercent),
		zap.String("priority_fee", ms.config.PriorityFee),
		zap.Uint32("compute_units", ms.config.ComputeUnits))

	// Продаем указанный процент токенов
	// SellPercentTokens будет запрашивать актуальный баланс внутри себя
	err := ms.config.DEX.SellPercentTokens(
		context.Background(),
		ms.config.TokenMint,
		percentToSell,
		ms.config.SlippagePercent,
		ms.config.PriorityFee,
		ms.config.ComputeUnits,
	)

	if err != nil {
		fmt.Printf("Error selling tokens: %v\n", err)
		return err
	}

	fmt.Println("Tokens sold successfully!")
	return nil
}

// onExitCommand вызывается при вводе команды выхода.
//
// Метод останавливает сессию мониторинга без продажи токенов
// и выводит соответствующее сообщение пользователю. Вызывается
// при вводе команд "q" или "exit".
//
// Параметры:
//   - _: строка команды (не используется)
//
// Возвращает:
//   - error: ошибку, если не удалось корректно остановить сессию
func (ms *MonitoringSession) onExitCommand(_ string) error {
	fmt.Println("\nExiting monitor mode without selling tokens.")
	ms.Stop()
	return nil
}
