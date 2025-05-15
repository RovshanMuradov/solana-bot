// =============================
// File: internal/dex/pumpswap/token_calc.go
// =============================

package pumpswap

import (
	"context"
	"errors"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"math"
	"strconv"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultSlippagePercent стандартное проскальзывание для расчетов
	DefaultSlippagePercent = 0.5 // 0.5% проскальзывание по умолчанию
	// DexFeePercent комиссия DEX при обмене токенов
	DexFeePercent = 0.25 // 0.25% комиссия PumpSwap DEX
	// MinPriceThreshold минимальная цена для предотвращения деления на ноль
	MinPriceThreshold = 1e-18 // Очень маленькое значение, близкое к нулю
)

// getPool получает актуальную информацию о пуле ликвидности, используя кэширование
func (d *DEX) getPool(ctx context.Context) (*PoolInfo, error) {
	// Если в кэше есть актуальная информация, возвращаем ее
	if d.cachedPool != nil && time.Since(d.cachedPoolTime) < d.cacheValidPeriod {
		d.logger.Debug("Using cached pool info",
			zap.String("pool", d.cachedPool.Address.String()),
			zap.Time("cached_at", d.cachedPoolTime))
		return d.cachedPool, nil
	}

	// Иначе получаем актуальную информацию о пуле
	effBase, effQuote := d.effectiveMints()
	pool, err := d.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 3, time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to find pool: %w", err)
	}

	// Обновляем кэш
	d.cachedPool = pool
	d.cachedPoolTime = time.Now()
	d.logger.Debug("Updated pool cache",
		zap.String("pool", pool.Address.String()),
		zap.Uint64("base_reserves", pool.BaseReserves),
		zap.Uint64("quote_reserves", pool.QuoteReserves))

	return pool, nil
}

// calculatePrice - внутренний метод для расчета цены на основе резервов пула
// baseReserves - резервы базового токена (BFI/нашего токена)
// quoteReserves - резервы quote токена (SOL/WSOL)
// baseDecimals - количество десятичных знаков у базового токена
// quoteDecimals - количество десятичных знаков у quote токена
func (d *DEX) calculatePrice(baseReserves, quoteReserves uint64, baseDecimals, quoteDecimals int) (float64, error) {
	if baseReserves == 0 {
		return MinPriceThreshold, errors.New("base reserves are zero, cannot calculate accurate price")
	}

	// Расчет цены с учетом десятичных знаков
	// Формула: (quoteReserves/baseReserves) * 10^(baseDecimals - quoteDecimals)
	price := float64(quoteReserves) / float64(baseReserves) *
		math.Pow10(baseDecimals-quoteDecimals)

	// Применяем нижнюю границу цены для предотвращения слишком малых значений
	if price < MinPriceThreshold {
		d.logger.Debug("Calculated price below minimum threshold, adjusting",
			zap.Float64("raw_price", price),
			zap.Float64("min_threshold", MinPriceThreshold))
		price = MinPriceThreshold
	}

	d.logger.Debug("Calculated token price",
		zap.Uint64("base_reserves", baseReserves),
		zap.Uint64("quote_reserves", quoteReserves),
		zap.Float64("price", price))

	return price, nil
}

// GetCurrentPrice returns the current token price in SOL, using pool reserves and caching.
func (d *DEX) GetCurrentPrice(ctx context.Context) (float64, error) {
	// Return cached price if still valid
	if time.Since(d.cachedPriceTime) < d.cacheValidPeriod {
		d.logger.Debug("cache hit for current price", zap.Float64("price", d.cachedPrice))
		return d.cachedPrice, nil
	}

	// Fetch latest pool info
	pool, err := d.getPool(ctx)
	if err != nil {
		return 0, err
	}

	// Determine decimals for base (BFI) and quote (WSOL)
	effBase, _ := d.effectiveMints()
	baseDecimals := int(d.getTokenDecimals(ctx, effBase, DefaultTokenDecimals))
	quoteDecimals := int(WSOLDecimals) // WSOL has 9 decimals

	// Calculate price using common method
	price, err := d.calculatePrice(pool.BaseReserves, pool.QuoteReserves, baseDecimals, quoteDecimals)
	if err != nil {
		d.logger.Warn("Price calculation warning", zap.Error(err))
		// Но продолжаем использовать полученную цену, даже если там были предупреждения
	}

	// Update cache
	d.cachedPrice = price
	d.cachedPriceTime = time.Now()
	d.logger.Debug("calculated current price", zap.Float64("price", price))

	return price, nil
}

// calculateExpectedSolOutput вычисляет ожидаемый выход SOL при продаже токенов
// с учетом формулы Constant Product AMM, комиссий DEX и проскальзывания
func (d *DEX) calculateExpectedSolOutput(ctx context.Context, tokenAmount float64, slippagePercent float64) (float64, error) {
	// Получаем пул
	pool, err := d.getPool(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get pool: %w", err)
	}

	if pool.BaseReserves == 0 || pool.QuoteReserves == 0 {
		return 0, errors.New("invalid pool reserves")
	}

	// Определяем десятичные знаки
	effBase, _ := d.effectiveMints()
	baseDecimals := d.getTokenDecimals(ctx, effBase, DefaultTokenDecimals)

	// Преобразуем токены в минимальные единицы
	tokenAmountRaw := uint64(tokenAmount * math.Pow10(int(baseDecimals)))

	// Формула Constant Product AMM для расчета выхода токенов:
	// ∆y = (y * ∆x) / (x + ∆x)
	// Где x - резервы базового токена, y - резервы quote токена, ∆x - количество продаваемых токенов
	baseReservesFloat := float64(pool.BaseReserves)
	quoteReservesFloat := float64(pool.QuoteReserves)
	tokenAmountFloat := float64(tokenAmountRaw)

	// Расчет выхода SOL (в минимальных единицах)
	solAmountRaw := (quoteReservesFloat * tokenAmountFloat) / (baseReservesFloat + tokenAmountFloat)

	// Учитываем комиссию DEX
	solAfterFees := solAmountRaw * (1.0 - (DexFeePercent / 100.0))

	// Применяем проскальзывание
	slippageFactor := 1.0 - (slippagePercent / 100.0)
	solAmountWithSlippage := solAfterFees * slippageFactor

	// Конвертируем в SOL
	solOutput := solAmountWithSlippage / math.Pow10(int(WSOLDecimals))

	d.logger.Debug("Calculated expected SOL output",
		zap.Float64("token_amount", tokenAmount),
		zap.Uint64("token_amount_raw", tokenAmountRaw),
		zap.Float64("sol_amount_raw", solAmountRaw),
		zap.Float64("sol_after_fees", solAfterFees),
		zap.Float64("dex_fee_percent", DexFeePercent),
		zap.Float64("slippage_percent", slippagePercent),
		zap.Float64("sol_output", solOutput))

	return solOutput, nil
}

// CalculatePnL computes profit and loss metrics for a given token amount and initial investment in SOL.
func (d *DEX) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	// 1. Get current price
	price, err := d.GetCurrentPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current price: %w", err)
	}

	// 2. Compute theoretical value in SOL (без учета комиссий и проскальзывания)
	theoreticalValue := tokenAmount * price

	// 3. Compute expected sell value with fees and slippage
	sellEstimate, err := d.calculateExpectedSolOutput(ctx, tokenAmount, DefaultSlippagePercent)
	if err != nil {
		d.logger.Warn("Error calculating sell estimate, using theoretical value", zap.Error(err))
		sellEstimate = theoreticalValue
	}

	// 4. Compute net PnL
	netPnL := sellEstimate - initialInvestment

	// 5. Compute PnL percentage
	pnlPercentage := 0.0
	if initialInvestment > 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100
	} else if netPnL > 0 {
		// Если начальная инвестиция 0, а PnL положительный, это бесконечный процент
		pnlPercentage = math.Inf(1)
	}

	// 6. Log detailed calculation info
	d.logger.Debug("PnL calculation completed",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("initial_investment", initialInvestment),
		zap.Float64("current_price", price),
		zap.Float64("theoretical_value", theoreticalValue),
		zap.Float64("sell_estimate_with_fees_and_slippage", sellEstimate),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	// 7. Build result
	result := &model.PnLResult{
		SellEstimate:      sellEstimate,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}

	return result, nil
}

// GetTokenPrice returns the current price of the given tokenMint (must be the base mint) in SOL.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Parse and verify mint
	effBase, _ := d.effectiveMints()
	mintKey, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}
	if !mintKey.Equals(effBase) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s", effBase, mintKey)
	}

	// Используем кэшированную цену, если доступна
	if time.Since(d.cachedPriceTime) < d.cacheValidPeriod && d.cachedPrice > 0 {
		d.logger.Debug("Using cached price for GetTokenPrice", zap.Float64("cached_price", d.cachedPrice))
		return d.cachedPrice, nil
	}

	// Retrieve pool reserves
	pool, err := d.getPool(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to find pool: %w", err)
	}

	// Determine decimals
	baseDecimals := int(d.getTokenDecimals(ctx, effBase, DefaultTokenDecimals))
	quoteDecimals := WSOLDecimals

	// Compute price using common method
	price, err := d.calculatePrice(pool.BaseReserves, pool.QuoteReserves, baseDecimals, quoteDecimals)
	if err != nil {
		d.logger.Warn("Price calculation warning in GetTokenPrice", zap.Error(err))
		// Но продолжаем использовать полученную цену, даже если там были предупреждения
	}

	// Update cache
	d.cachedPrice = price
	d.cachedPriceTime = time.Now()

	return price, nil
}

// GetTokenBalance получает баланс токена в кошельке пользователя.
func (d *DEX) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// Проверка токена на соответствие с тем, что используется в DEX
	effBase, _ := d.effectiveMints()
	mintKey, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}
	if !mintKey.Equals(effBase) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s", effBase, mintKey)
	}

	// Находим ATA адрес для токена
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.QuoteMint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Сначала пробуем с Processed commitment для скорости
	commitmentLevel := rpc.CommitmentProcessed
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)

	// При неудаче пробуем с Confirmed commitment
	if err != nil {
		d.logger.Debug("Failed to get balance with Processed commitment, trying Confirmed",
			zap.String("token_mint", d.config.QuoteMint.String()),
			zap.String("user_ata", userATA.String()),
			zap.Error(err))

		// Повторный запрос с более надежным уровнем подтверждения
		commitmentLevel = rpc.CommitmentConfirmed
		result, err = d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)
	}

	// Проверяем ошибку после возможной повторной попытки
	if err != nil {
		return 0, fmt.Errorf("failed to get token account balance: %w", err)
	}

	if result == nil || result.Value.Amount == "" {
		return 0, fmt.Errorf("no token balance found")
	}

	// Парсим результат в uint64
	balance, err := strconv.ParseUint(result.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token balance: %w", err)
	}

	d.logger.Debug("Got token balance",
		zap.Uint64("balance", balance),
		zap.String("token_mint", tokenMint),
		zap.String("user_ata", userATA.String()),
		zap.String("commitment", string(commitmentLevel)))

	return balance, nil
}

// SellPercentTokens продает указанный процент от доступного баланса токенов.
// Метод получает баланс токена, рассчитывает сумму для продажи в соответствии
// с указанным процентом и выполняет операцию продажи.
func (d *DEX) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	// Проверка токена на соответствие с тем, что используется в DEX
	effBase, _ := d.effectiveMints()
	mintKey, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return fmt.Errorf("invalid token mint: %w", err)
	}
	if !mintKey.Equals(effBase) {
		return fmt.Errorf("token mint mismatch: expected %s, got %s", effBase, mintKey)
	}

	// Проверка валидности параметра percentToSell
	if percentToSell <= 0 || percentToSell > 100 {
		return fmt.Errorf("percentToSell должен быть в пределах от 0 до 100, получено: %f", percentToSell)
	}

	// Получаем текущий баланс токена
	tokenBalance, err := d.GetTokenBalance(ctx, tokenMint)
	if err != nil {
		return fmt.Errorf("не удалось получить баланс токена: %w", err)
	}

	// Проверяем, есть ли токены для продажи
	if tokenBalance == 0 {
		return fmt.Errorf("нет токенов для продажи")
	}

	// Рассчитываем количество токенов для продажи
	amountToSell := uint64(float64(tokenBalance) * percentToSell / 100.0)

	// Убедимся, что продаём хотя бы 1 токен, если есть баланс
	if amountToSell == 0 && tokenBalance > 0 {
		amountToSell = 1
	}

	d.logger.Info("Продажа токенов",
		zap.Uint64("current_balance", tokenBalance),
		zap.Float64("percent_to_sell", percentToSell),
		zap.Uint64("amount_to_sell", amountToSell),
		zap.Float64("slippage_percent", slippagePercent))

	// Выполняем продажу указанного количества токенов
	return d.ExecuteSell(ctx, amountToSell, slippagePercent, priorityFeeSol, computeUnits)
}
