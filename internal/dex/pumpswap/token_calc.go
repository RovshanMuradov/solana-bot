// =============================
// File: internal/dex/pumpswap/token_calc.go
// =============================

package pumpswap

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"

	"go.uber.org/zap"
)

// PnLCalculatorInterface определяет интерфейс для расчета прибыли/убытка и стоимости токенов
type PnLCalculatorInterface interface {
}

// TokenPnL содержит данные о прибыли/убытке для токена
type TokenPnL struct {
	CurrentPrice      float64 // Текущая цена токена (SOL за токен)
	TheoreticalValue  float64 // Теоретическая стоимость: токены * CurrentPrice
	SellEstimate      float64 // Приблизительная выручка при продаже (с учетом комиссии и проскальзывания)
	InitialInvestment float64 // Первоначальные вложения в SOL
	NetPnL            float64 // Чистая прибыль/убыток: SellEstimate - InitialInvestment
	PnLPercentage     float64 // Процент PnL от начальных вложений
}

// SellEstimate содержит данные о предполагаемом результате продажи токенов
type SellEstimate struct {
	InputTokens float64 // Количество токенов, которые продаем
	OutputSOL   float64 // Ожидаемое количество SOL при продаже
	PriceImpact float64 // Влияние на цену в процентах
	Price       float64 // Цена исполнения сделки
	Fee         float64 // Общая комиссия в SOL
	MinimumOut  float64 // Минимальный выход с учетом проскальзывания
}

// TokenCalculator реализует расчеты для PumpSwap
type TokenCalculator struct {
	dex         *DEX
	logger      *zap.Logger
	poolManager PoolManagerInterface
	config      *Config

	// Кэшированные данные для оптимизации запросов
	cachedPool       *PoolInfo
	cachedPoolTime   time.Time
	cachedPrice      float64
	cachedPriceTime  time.Time
	cacheValidPeriod time.Duration
}

// NewTokenCalculator создает новый экземпляр калькулятора для PumpSwap
func NewTokenCalculator(dex *DEX, poolManager PoolManagerInterface, config *Config, logger *zap.Logger) *TokenCalculator {
	return &TokenCalculator{
		dex:              dex,
		poolManager:      poolManager,
		config:           config,
		logger:           logger.Named("token_calculator"),
		cacheValidPeriod: 30 * time.Second, // Кэш валиден 30 секунд по умолчанию
	}
}

// getPool получает актуальную информацию о пуле ликвидности, используя кэширование
func (tc *TokenCalculator) getPool(ctx context.Context) (*PoolInfo, error) {
	// Если в кэше есть актуальная информация, возвращаем ее
	if tc.cachedPool != nil && time.Since(tc.cachedPoolTime) < tc.cacheValidPeriod {
		tc.logger.Debug("Using cached pool info",
			zap.String("pool", tc.cachedPool.Address.String()),
			zap.Time("cached_at", tc.cachedPoolTime))
		return tc.cachedPool, nil
	}

	// Иначе получаем актуальную информацию о пуле
	effBase, effQuote := tc.dex.effectiveMints()
	pool, err := tc.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 3, time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to find pool: %w", err)
	}

	// Обновляем кэш
	tc.cachedPool = pool
	tc.cachedPoolTime = time.Now()
	tc.logger.Debug("Updated pool cache",
		zap.String("pool", pool.Address.String()),
		zap.Uint64("base_reserves", pool.BaseReserves),
		zap.Uint64("quote_reserves", pool.QuoteReserves))

	return pool, nil
}

// GetCurrentPrice возвращает текущую цену токена
func (tc *TokenCalculator) GetCurrentPrice(ctx context.Context) (float64, error) {
	// Если в кэше есть актуальная информация о цене, возвращаем ее
	if tc.cachedPrice > 0 && time.Since(tc.cachedPriceTime) < tc.cacheValidPeriod {
		tc.logger.Debug("Using cached price", zap.Float64("price", tc.cachedPrice))
		return tc.cachedPrice, nil
	}

	// Получаем актуальную информацию о пуле
	pool, err := tc.getPool(ctx)
	if err != nil {
		return 0, err
	}

	// Вычисляем цену по формуле: quote_reserves / base_reserves * (10^base_decimals / 10^quote_decimals)
	effBase, _ := tc.dex.effectiveMints()
	baseDecimals := tc.dex.getTokenDecimals(ctx, effBase, DefaultTokenDecimals)
	quoteDecimals := uint8(WSOLDecimals) // WSOL всегда имеет 9 знаков после запятой

	// Исключаем деление на ноль
	if pool.BaseReserves == 0 {
		return 0, fmt.Errorf("base reserves are zero, cannot calculate price")
	}

	// Расчет с использованием big.Float для предотвращения потери точности
	baseReserves := new(big.Float).SetUint64(pool.BaseReserves)
	quoteReserves := new(big.Float).SetUint64(pool.QuoteReserves)
	quotient := new(big.Float).Quo(quoteReserves, baseReserves)

	// Корректировка с учетом разных десятичных знаков
	decimalAdjustment := math.Pow10(int(quoteDecimals) - int(baseDecimals))
	adjustedPrice := new(big.Float).Mul(quotient, big.NewFloat(decimalAdjustment))

	price, _ := adjustedPrice.Float64()

	// Обновляем кэш цены
	tc.cachedPrice = price
	tc.cachedPriceTime = time.Now()
	tc.logger.Debug("Current price calculated", zap.Float64("price", price))

	return price, nil
}

// CalculatePnL вычисляет показатели прибыли и убытка для токенов
func (tc *TokenCalculator) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*TokenPnL, error) {
	// Получаем текущую цену токена
	currentPrice, err := tc.GetCurrentPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current price: %w", err)
	}

	// Теоретическая стоимость: количество токенов * текущая цена
	theoreticalValue := tokenAmount * currentPrice

	// Оценка продажи (с учетом проскальзывания и комиссий)
	sellEstimate, err := tc.EstimateSellOutput(ctx, tokenAmount, 1.0) // Используем 1% проскальзывание по умолчанию
	if err != nil {
		return nil, fmt.Errorf("failed to estimate sell output: %w", err)
	}

	// Чистая прибыль/убыток и процент
	netPnL := sellEstimate.OutputSOL - initialInvestment
	var pnlPercentage float64
	if initialInvestment > 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100.0
	}

	// Результат расчета
	result := &TokenPnL{
		CurrentPrice:      currentPrice,
		TheoreticalValue:  theoreticalValue,
		SellEstimate:      sellEstimate.OutputSOL,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}

	tc.logger.Debug("PnL calculation result",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("current_price", currentPrice),
		zap.Float64("theoretical_value", theoreticalValue),
		zap.Float64("sell_estimate", sellEstimate.OutputSOL),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	return result, nil
}

// EstimateSellOutput вычисляет ожидаемый выход при продаже токенов
func (tc *TokenCalculator) EstimateSellOutput(ctx context.Context, tokenAmount float64, slippagePercent float64) (*SellEstimate, error) {
	// Получаем актуальную информацию о пуле
	pool, err := tc.getPool(ctx)
	if err != nil {
		return nil, err
	}

	// Получаем количество десятичных знаков для токена
	effBase, _ := tc.dex.effectiveMints()
	baseDecimals := tc.dex.getTokenDecimals(ctx, effBase, DefaultTokenDecimals)

	// Конвертируем human-readable количество токенов в наименьшие единицы
	tokenAmountRaw := uint64(tokenAmount * math.Pow10(int(baseDecimals)))

	// Рассчитываем fee factor (1 - комиссия)
	lpFeeFactor := float64(pool.FeesBasisPoints) / 10000.0
	protocolFeeFactor := float64(pool.ProtocolFeeBPS) / 10000.0
	totalFeeFactor := 1.0 - (lpFeeFactor + protocolFeeFactor)

	// Вычисляем выход по формуле: y * dx * feeFactor / (x + dx * feeFactor)
	// где y - резервы WSOL, x - резервы токена, dx - количество токенов на продажу
	xReserves := new(big.Float).SetUint64(pool.BaseReserves)
	yReserves := new(big.Float).SetUint64(pool.QuoteReserves)
	dx := new(big.Float).SetUint64(tokenAmountRaw)

	// Применяем комиссию к входному количеству
	dxWithFee := new(big.Float).Mul(dx, big.NewFloat(totalFeeFactor))

	// Числитель: y * dx * feeFactor
	numerator := new(big.Float).Mul(yReserves, dxWithFee)

	// Знаменатель: x + dx * feeFactor
	denominator := new(big.Float).Add(xReserves, dxWithFee)

	// Результат: y * dx * feeFactor / (x + dx * feeFactor)
	result := new(big.Float).Quo(numerator, denominator)

	outputRaw, _ := result.Uint64()

	// Конвертируем обратно в human-readable SOL
	outputSOL := float64(outputRaw) / math.Pow10(int(WSOLDecimals))

	// Рассчитываем цену исполнения
	var execPrice float64
	if tokenAmount > 0 {
		execPrice = outputSOL / tokenAmount
	}

	// Рассчитываем комиссию в SOL
	totalFeeSOL := outputSOL * (lpFeeFactor + protocolFeeFactor) / totalFeeFactor

	// Рассчитываем минимальный выход с учетом проскальзывания
	minOutputSOL := outputSOL * (1.0 - slippagePercent/100.0)

	// Рассчитываем влияние на цену
	currentPrice, err := tc.GetCurrentPrice(ctx)
	if err != nil {
		// Если не можем получить текущую цену, пропускаем расчет price impact
		tc.logger.Warn("Could not calculate price impact", zap.Error(err))
	}

	var priceImpact float64
	if currentPrice > 0 {
		// Price impact = (currentPrice - execPrice) / currentPrice * 100
		priceImpact = (currentPrice - execPrice) / currentPrice * 100.0
	}

	// Составляем результат
	estimate := &SellEstimate{
		InputTokens: tokenAmount,
		OutputSOL:   outputSOL,
		PriceImpact: priceImpact,
		Price:       execPrice,
		Fee:         totalFeeSOL,
		MinimumOut:  minOutputSOL,
	}

	tc.logger.Debug("Sell estimate calculated",
		zap.Float64("input_tokens", tokenAmount),
		zap.Float64("output_sol", outputSOL),
		zap.Float64("price_impact", priceImpact),
		zap.Float64("execution_price", execPrice),
		zap.Float64("fee", totalFeeSOL),
		zap.Float64("minimum_out", minOutputSOL))

	return estimate, nil
}

// EstimateBuyOutput вычисляет ожидаемый выход при покупке токенов за WSOL
func (tc *TokenCalculator) EstimateBuyOutput(ctx context.Context, solAmount float64, slippagePercent float64) (*BuyEstimate, error) {
	// Получаем актуальную информацию о пуле
	pool, err := tc.getPool(ctx)
	if err != nil {
		return nil, err
	}

	// Получаем количество десятичных знаков для токена
	effBase, _ := tc.dex.effectiveMints()
	baseDecimals := tc.dex.getTokenDecimals(ctx, effBase, DefaultTokenDecimals)

	// Конвертируем human-readable количество SOL в наименьшие единицы (lamports)
	solAmountRaw := uint64(solAmount * math.Pow10(int(WSOLDecimals)))

	// Рассчитываем fee factor (1 - комиссия)
	lpFeeFactor := float64(pool.FeesBasisPoints) / 10000.0
	protocolFeeFactor := float64(pool.ProtocolFeeBPS) / 10000.0
	totalFeeFactor := 1.0 - (lpFeeFactor + protocolFeeFactor)

	// Вычисляем выход по формуле: x * dy * feeFactor / (y + dy * feeFactor)
	// где x - резервы токена, y - резервы WSOL, dy - количество WSOL на покупку
	xReserves := new(big.Float).SetUint64(pool.BaseReserves)
	yReserves := new(big.Float).SetUint64(pool.QuoteReserves)
	dy := new(big.Float).SetUint64(solAmountRaw)

	// Применяем комиссию к входному количеству
	dyWithFee := new(big.Float).Mul(dy, big.NewFloat(totalFeeFactor))

	// Числитель: x * dy * feeFactor
	numerator := new(big.Float).Mul(xReserves, dyWithFee)

	// Знаменатель: y + dy * feeFactor
	denominator := new(big.Float).Add(yReserves, dyWithFee)

	// Результат: x * dy * feeFactor / (y + dy * feeFactor)
	result := new(big.Float).Quo(numerator, denominator)

	outputRaw, _ := result.Uint64()

	// Конвертируем обратно в human-readable tokens
	outputTokens := float64(outputRaw) / math.Pow10(int(baseDecimals))

	// Рассчитываем цену исполнения
	var execPrice float64
	if outputTokens > 0 {
		execPrice = solAmount / outputTokens
	}

	// Рассчитываем комиссию в SOL
	totalFeeSol := solAmount * (lpFeeFactor + protocolFeeFactor)

	// Рассчитываем минимальный выход с учетом проскальзывания
	minOutputTokens := outputTokens * (1.0 - slippagePercent/100.0)

	// Рассчитываем влияние на цену
	currentPrice, err := tc.GetCurrentPrice(ctx)
	if err != nil {
		// Если не можем получить текущую цену, пропускаем расчет price impact
		tc.logger.Warn("Could not calculate price impact", zap.Error(err))
	}

	var priceImpact float64
	if currentPrice > 0 {
		// Price impact = (execPrice - currentPrice) / currentPrice * 100
		priceImpact = (execPrice - currentPrice) / currentPrice * 100.0
	}

	// Составляем результат
	estimate := &BuyEstimate{
		InputSOL:     solAmount,
		OutputTokens: outputTokens,
		PriceImpact:  priceImpact,
		Price:        execPrice,
		Fee:          totalFeeSol,
		MinimumOut:   minOutputTokens,
	}

	tc.logger.Debug("Buy estimate calculated",
		zap.Float64("input_sol", solAmount),
		zap.Float64("output_tokens", outputTokens),
		zap.Float64("price_impact", priceImpact),
		zap.Float64("execution_price", execPrice),
		zap.Float64("fee", totalFeeSol),
		zap.Float64("minimum_out", minOutputTokens))

	return estimate, nil
}

// BuyEstimate содержит данные о предполагаемом результате покупки токенов
type BuyEstimate struct {
	InputSOL     float64 // Количество SOL, которые тратим
	OutputTokens float64 // Ожидаемое количество токенов при покупке
	PriceImpact  float64 // Влияние на цену в процентах
	Price        float64 // Цена исполнения сделки
	Fee          float64 // Общая комиссия в SOL
	MinimumOut   float64 // Минимальный выход с учетом проскальзывания
}
