// =============================
// File: internal/dex/pumpswap/token_calc.go
// =============================

package pumpswap

import (
	"context"
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

// calculateEstimate возвращает прогнозный выход SOL за tokenAmount,
// с учётом только протокольной/DEX-комиссии.
func (d *DEX) calculateEstimate(ctx context.Context, tokenAmount float64, reserves interface{}) (float64, error) {
	// Приводим reserves к типу PoolInfo
	pool, ok := reserves.(*PoolInfo)
	if !ok {
		return 0, fmt.Errorf("invalid reserves type for pumpswap: %T", reserves)
	}

	if pool.BaseReserves == 0 || pool.QuoteReserves == 0 {
		d.logger.Warn("Invalid pool reserves, using zero estimate",
			zap.Uint64("base_reserves", pool.BaseReserves),
			zap.Uint64("quote_reserves", pool.QuoteReserves))
		return 0, nil
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

	// Конвертируем в SOL
	solOutput := solAfterFees / math.Pow10(int(WSOLDecimals))

	d.logger.Debug("Calculated expected SOL output",
		zap.Float64("token_amount", tokenAmount),
		zap.Uint64("token_amount_raw", tokenAmountRaw),
		zap.Float64("sol_amount_raw", solAmountRaw),
		zap.Float64("sol_after_fees", solAfterFees),
		zap.Float64("dex_fee_percent", DexFeePercent),
		zap.Float64("sol_output", solOutput))

	return solOutput, nil
}

// GetTokenPrice возвращает текущую цену токена в SOL, используя данные о резервах пула
// и кэширование для оптимизации производительности.
// Для совместимости с интерфейсом DEX принимает tokenMint, но не использует его
// так как мы уже храним эту информацию в конфигурации DEX.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Используем кэшированную цену, если она актуальна
	if time.Since(d.cachedPriceTime) < d.cacheValidPeriod {
		d.logger.Debug("Using cached price", zap.Float64("price", d.cachedPrice))
		return d.cachedPrice, nil
	}

	// Получаем актуальную информацию о пуле
	pool, err := d.getPool(ctx)
	if err != nil {
		return 0, err
	}

	// Определяем десятичные знаки для базового токена и WSOL
	effBase, _ := d.effectiveMints()
	baseDecimals := int(d.getTokenDecimals(ctx, effBase, DefaultTokenDecimals))
	quoteDecimals := int(WSOLDecimals) // WSOL имеет 9 десятичных знаков

	// Проверяем, что у нас есть валидные резервы
	if pool.BaseReserves == 0 {
		d.logger.Warn("Base reserves are zero, using minimum price threshold")
		d.cachedPrice = MinPriceThreshold
		d.cachedPriceTime = time.Now()
		return MinPriceThreshold, nil
	}

	// Расчет цены с учетом десятичных знаков
	// Формула: (quoteReserves/baseReserves) * 10^(baseDecimals - quoteDecimals)
	price := float64(pool.QuoteReserves) / float64(pool.BaseReserves) *
		math.Pow10(baseDecimals-quoteDecimals)

	// Применяем нижнюю границу цены для предотвращения слишком малых значений
	if price < MinPriceThreshold {
		d.logger.Debug("Calculated price below minimum threshold, adjusting",
			zap.Float64("raw_price", price),
			zap.Float64("min_threshold", MinPriceThreshold))
		price = MinPriceThreshold
	}

	// Обновляем кэш
	d.cachedPrice = price
	d.cachedPriceTime = time.Now()
	d.logger.Debug("Updated price cache",
		zap.Float64("price", price),
		zap.Uint64("base_reserves", pool.BaseReserves),
		zap.Uint64("quote_reserves", pool.QuoteReserves))

	return price, nil
}

// GetTokenBalance получает баланс токена в кошельке пользователя.
// Для совместимости с интерфейсом DEX принимает tokenMint, но не использует его
// так как мы уже храним эту информацию в конфигурации DEX.
func (d *DEX) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// Получаем информацию о токене
	effBase, _ := d.effectiveMints()

	// Находим ATA адрес для токена
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, effBase)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Сначала пробуем с Processed commitment для скорости
	commitmentLevel := rpc.CommitmentProcessed
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)

	// При неудаче пробуем с Confirmed commitment
	if err != nil {
		d.logger.Debug("Failed to get balance with Processed commitment, trying Confirmed",
			zap.String("token_mint", effBase.String()),
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
		zap.String("token_mint", effBase.String()),
		zap.String("user_ata", userATA.String()),
		zap.String("commitment", string(commitmentLevel)))

	return balance, nil
}

// CalculatePnL вычисляет метрики прибыли и убытков для заданного количества токенов
// и начальной инвестиции в SOL. Учитывает только комиссию DEX.
func (d *DEX) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	// 1. Из начальной инвестиции вычитаем комиссию при покупке
	buyFee := initialInvestment * (DexFeePercent / 100.0)
	costBasis := initialInvestment - buyFee

	// 2. Получаем пул
	pool, err := d.getPool(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool: %w", err)
	}

	// 3. Вычисляем ожидаемую стоимость продажи с учетом комиссий DEX
	sellEstimate, err := d.calculateEstimate(ctx, tokenAmount, pool)
	if err != nil {
		d.logger.Warn("Error calculating sell estimate", zap.Error(err))
		return nil, fmt.Errorf("failed to calculate sell estimate: %w", err)
	}

	// Расчет чистой прибыли/убытка

	// 5. Вычисляем чистую прибыль/убыток относительно скорректированной начальной инвестиции
	netPnL := sellEstimate - costBasis

	// 6. Вычисляем процент прибыли/убытка
	pnlPercentage := 0.0
	if costBasis > 0 {
		pnlPercentage = (netPnL / costBasis) * 100
	} else if netPnL > 0 {
		// Если начальная инвестиция 0, а PnL положительный, это бесконечный процент
		pnlPercentage = math.Inf(1)
	}

	// 7. Логируем детальную информацию о расчетах
	d.logger.Debug("PnL calculation completed",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("initial_investment", initialInvestment),
		zap.Float64("buy_fee", buyFee),
		zap.Float64("cost_basis", costBasis),
		zap.Float64("sell_estimate", sellEstimate),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	// 8. Формируем результат
	result := &model.PnLResult{
		SellEstimate:      sellEstimate,
		InitialInvestment: costBasis, // Теперь возвращаем уже "чистую" сумму
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}

	return result, nil
}

// SellPercentTokens продает указанный процент от доступного баланса токенов.
// Метод получает баланс токена, рассчитывает сумму для продажи в соответствии
// с указанным процентом и выполняет операцию продажи.
// Для совместимости с интерфейсом DEX принимает tokenMint, но не использует его
// так как мы уже храним эту информацию в конфигурации DEX.
func (d *DEX) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64,
	slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
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
		zap.Uint64("amount_to_sell", amountToSell))

	// Выполняем продажу указанного количества токенов
	return d.executeSell(ctx, amountToSell, slippagePercent, priorityFeeSol, computeUnits)
}
