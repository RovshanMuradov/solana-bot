package monitor

import (
	"context"
	"math"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// PriceUpdateCallback - функция обратного вызова, вызываемая при обновлении цены токена.
//
// Функция предоставляет информацию о текущей цене токена, начальной цене, процентном изменении
// и количестве токенов. Используется для уведомления о значимых изменениях цены и актуализации
// данных в пользовательском интерфейсе.
//
// Параметры:
//   - currentPriceSol: текущая цена токена в SOL
//   - initialPriceSol: начальная цена токена в SOL (на момент начала мониторинга)
//   - percentChange: процентное изменение цены относительно начальной (может быть положительным или отрицательным)
//   - tokenAmount: количество токенов, отслеживаемых монитором
type PriceUpdateCallback func(currentPriceSol float64, initialPriceSol float64, percentChange float64, tokenAmount float64)

// PriceMonitor отслеживает изменения цены токена.
//
// Структура периодически запрашивает текущую цену токена через интерфейс DEX,
// вычисляет процентное изменение относительно начальной цены и уведомляет
// о результатах через функцию обратного вызова. Поддерживает отмену операций
// через контекст и структурированное логирование.
//
// Поля:
//   - dex: интерфейс DEX для получения цены токена
//   - interval: интервал между проверками цены
//   - initialPrice: начальная цена токена при запуске мониторинга
//   - tokenAmount: количество приобретенных токенов
//   - tokenMint: адрес минта токена
//   - initialAmount: начальная сумма SOL, потраченная на покупку
//   - logger: логгер для записи информации и ошибок
//   - callback: функция обратного вызова для обновлений цены
//   - ctx: контекст для отмены операций
//   - cancel: функция для отмены контекста
type PriceMonitor struct {
	dex           dex.DEX             // DEX interface for price retrieval
	interval      time.Duration       // Interval between price checks
	initialPrice  float64             // Initial token price when monitoring started
	tokenAmount   float64             // Amount of tokens purchased
	tokenMint     string              // Token mint address
	initialAmount float64             // Initial SOL amount spent
	logger        *zap.Logger         // Logger
	callback      PriceUpdateCallback // Callback for price updates
	ctx           context.Context     // Context for cancellation
	cancel        context.CancelFunc  // Cancel function
}

// NewPriceMonitor создает новый монитор цены токена.
//
// Функция инициализирует структуру PriceMonitor, создает новый контекст
// с функцией отмены и настраивает все необходимые параметры для отслеживания
// изменений цены токена.
//
// Параметры:
//   - dex: интерфейс DEX для получения цены токена
//   - tokenMint: адрес минта отслеживаемого токена
//   - initialPrice: начальная цена токена в SOL
//   - tokenAmount: количество приобретенных токенов
//   - initialAmount: начальная сумма SOL, потраченная на покупку
//   - interval: интервал между проверками цены
//   - logger: логгер для записи информации и ошибок
//   - callback: функция обратного вызова для уведомлений об изменении цены
//
// Возвращает:
//   - *PriceMonitor: новый экземпляр монитора цены токена
func NewPriceMonitor(dex dex.DEX, tokenMint string, initialPrice float64,
	tokenAmount float64, initialAmount float64,
	interval time.Duration, logger *zap.Logger,
	callback PriceUpdateCallback) *PriceMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &PriceMonitor{
		dex:           dex,
		interval:      interval,
		initialPrice:  initialPrice,
		tokenAmount:   tokenAmount,
		tokenMint:     tokenMint,
		initialAmount: initialAmount,
		logger:        logger,
		callback:      callback,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start запускает процесс мониторинга цены.
//
// Метод сначала выполняет немедленное обновление цены, а затем запускает
// циклическую проверку цены с заданным интервалом. Мониторинг продолжается
// до тех пор, пока контекст не будет отменен через метод Stop.
//
// При каждом обновлении цены вызывается функция обратного вызова,
// предоставляя актуальную информацию о цене и процентном изменении.
func (pm *PriceMonitor) Start() {
	pm.logger.Info("Starting price monitor",
		zap.String("token_mint", pm.tokenMint),
		zap.Float64("initial_price", pm.initialPrice),
		zap.Duration("interval", pm.interval))

	// Run the first update immediately
	pm.updatePrice()

	// Start the ticker for periodic updates
	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.updatePrice()
		case <-pm.ctx.Done():
			pm.logger.Debug("Price monitor stopped")
			return
		}
	}
}

// Stop останавливает мониторинг цены.
//
// Метод отменяет контекст, что приводит к завершению горутины,
// отслеживающей изменения цены. Это безопасный способ остановки
// монитора, гарантирующий корректное завершение всех внутренних процессов.
//
// Метод безопасен для многократного вызова и при вызове на
// уже остановленном мониторе не производит никаких действий.
func (pm *PriceMonitor) Stop() {
	if pm.cancel != nil {
		pm.cancel()
	}
}

// updatePrice получает текущую цену токена и вызывает функцию обратного вызова.
//
// Метод запрашивает текущую цену токена у DEX, вычисляет процентное изменение
// относительно начальной цены и вызывает зарегистрированную функцию обратного
// вызова с актуальными данными. В случае ошибки при получении цены, ошибка
// логируется, но выполнение мониторинга не прерывается.
//
// Особенности работы:
//   - Для запроса цены используется таймаут 10 секунд
//   - Процентное изменение округляется до 2 знаков после запятой
//   - Если начальная цена равна 0, процентное изменение также будет 0
func (pm *PriceMonitor) updatePrice() {
	ctx, cancel := context.WithTimeout(pm.ctx, 10*time.Second)
	defer cancel()

	currentPrice, err := pm.dex.GetTokenPrice(ctx, pm.tokenMint)
	if err != nil {
		pm.logger.Error("Failed to get token price", zap.Error(err))
		return
	}

	// Calculate percent change
	percentChange := 0.0
	if pm.initialPrice > 0 {
		percentChange = ((currentPrice - pm.initialPrice) / pm.initialPrice) * 100
	} // TODO: work with balance calculation

	// Format to 2 decimal places
	percentChange = math.Floor(percentChange*100) / 100

	// Call the callback with price information
	if pm.callback != nil {
		pm.callback(currentPrice, pm.initialPrice, percentChange, pm.tokenAmount)
	}
}

// SetCallback устанавливает функцию обратного вызова для обновлений цены.
//
// Метод позволяет изменить функцию обратного вызова, которая будет вызываться
// при каждом обновлении цены токена. Может использоваться для динамической
// смены обработчика уведомлений об изменении цены.
//
// Параметры:
//   - callback: новая функция обратного вызова для обновлений цены
func (pm *PriceMonitor) SetCallback(callback PriceUpdateCallback) {
	pm.callback = callback
}
