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
	"math"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// PnLCalculatorInterface определяет интерфейс для расчета прибыли/убытка и стоимости токенов
type PnLCalculatorInterface interface {
	//GetTokenPrice(ctx context.Context, tokenMint string) (float64, error)
	CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*TokenPnL, error)
	//GetTokenBalance(ctx context.Context, commitment ...rpc.CommitmentType) (uint64, error)
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

// GetCurrentPrice returns the current BFI token price in SOL, using pool reserves and caching.
func (tc *TokenCalculator) GetCurrentPrice(ctx context.Context) (float64, error) {
	// Return cached price if still valid
	if time.Since(tc.cachedPriceTime) < tc.cacheValidPeriod {
		tc.logger.Debug("cache hit for current price", zap.Float64("price", tc.cachedPrice))
		return tc.cachedPrice, nil
	}

	// Fetch latest pool info
	pool, err := tc.getPool(ctx)
	if err != nil {
		return 0, err
	}
	if pool.BaseReserves == 0 {
		return 0, errors.New("base reserves are zero, cannot calculate price")
	}

	// Determine decimals for base (BFI) and quote (WSOL)
	effBase, _ := tc.dex.effectiveMints()
	baseDecimals := tc.dex.getTokenDecimals(ctx, effBase, DefaultTokenDecimals)
	quoteDecimals := WSOLDecimals // WSOL has 9 decimals

	// Calculate price = (quoteReserves/baseReserves) * 10^(baseDecimals - quoteDecimals)
	price := float64(pool.QuoteReserves) /
		float64(pool.BaseReserves) *
		math.Pow10(int(baseDecimals)-int(quoteDecimals))

	// Update cache
	tc.cachedPrice = price
	tc.cachedPriceTime = time.Now()
	tc.logger.Debug("calculated current price", zap.Float64("price", price))

	return price, nil
}

// CalculatePnL computes profit and loss metrics for a given token amount and initial investment in SOL.
func (tc *TokenCalculator) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*TokenPnL, error) {
	// 1. Get current price
	price, err := tc.GetCurrentPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current price: %w", err)
	}

	// 2. Compute current value in SOL
	currentValue := tokenAmount * price

	// 3. Compute net PnL
	netPnL := currentValue - initialInvestment

	// 4. Compute PnL percentage
	pnlPercentage := 0.0
	if initialInvestment != 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100
	}

	// 5. Build result
	result := &TokenPnL{
		CurrentPrice:      price,
		TheoreticalValue:  currentValue,
		SellEstimate:      currentValue,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}

	return result, nil
}

// GetTokenPrice returns the current price of the given tokenMint (must be the base mint) in SOL.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Parse and verify mint
	effBase, effQuote := d.effectiveMints()
	mintKey, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}
	if !mintKey.Equals(effBase) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s", effBase, mintKey)
	}

	// Retrieve pool reserves
	pool, err := d.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 3, time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to find pool: %w", err)
	}
	if pool.BaseReserves == 0 {
		return 0, fmt.Errorf("base reserves are zero, cannot calculate price")
	}

	// Determine decimals
	baseDecimals := d.getTokenDecimals(ctx, effBase, DefaultTokenDecimals)
	quoteDecimals := WSOLDecimals

	// Compute price
	price := float64(pool.QuoteReserves) /
		float64(pool.BaseReserves) *
		math.Pow10(int(baseDecimals)-int(quoteDecimals))

	return price, nil
}

// GetTokenBalance получает баланс токена в кошельке пользователя.
func (d *DEX) GetTokenBalance(ctx context.Context, commitment ...rpc.CommitmentType) (uint64, error) {
	// Находим ATA адрес для токена
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.QuoteMint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Шаг 2: Определение уровня подтверждения (commitment level)
	// По умолчанию используем Processed - самый быстрый уровень
	// Можно переопределить через вариативный параметр
	commitmentLevel := rpc.CommitmentProcessed
	if len(commitment) > 0 {
		commitmentLevel = commitment[0]
	}

	// Получаем баланс токена
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)

	// Шаг 4: При неудаче с Processed, пробуем Confirmed
	if err != nil && commitmentLevel == rpc.CommitmentProcessed {
		d.logger.Debug("Failed to get balance with Processed commitment, trying Confirmed",
			zap.String("token_mint", d.config.QuoteMint.String()),
			zap.String("user_ata", userATA.String()),
			zap.Error(err))

		// Повторный запрос с более надежным уровнем подтверждения
		result, err = d.client.GetTokenAccountBalance(ctx, userATA, rpc.CommitmentConfirmed)
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

	return balance, nil
}

// SellPercentTokens продает указанный процент от доступного баланса токенов.
// Метод получает баланс токена, рассчитывает сумму для продажи в соответствии
// с указанным процентом и выполняет операцию продажи.
func (d *DEX) SellPercentTokens(ctx context.Context, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	// Проверка валидности параметра percentToSell
	if percentToSell <= 0 || percentToSell > 100 {
		return fmt.Errorf("percentToSell должен быть в пределах от 0 до 100, получено: %f", percentToSell)
	}

	// Получаем текущий баланс токена
	tokenBalance, err := d.GetTokenBalance(ctx)
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
