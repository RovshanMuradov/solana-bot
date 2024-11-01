// internal/dex/raydium/pool.go
package raydium

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// TODO: pool.go
// - Добавить поддержку новых типов пулов
// - Реализовать методы для работы с concentrated liquidity
// - Добавить методы для анализа ликвидности

// PoolManager управляет операциями с пулом ликвидности
type PoolManager struct {
	client blockchain.Client
	logger *zap.Logger
}

// NewPoolManager создает новый менеджер пула
func NewPoolManager(client blockchain.Client, logger *zap.Logger) *PoolManager {
	return &PoolManager{
		client: client,
		logger: logger.Named("pool-manager"),
	}
}

// Добавляем метод для загрузки состояния lookup table в PoolManager
func (pm *PoolManager) LoadPoolLookupTable(
	ctx context.Context,
	pool *RaydiumPool,
) error {
	if pool.LookupTableID.IsZero() {
		return nil
	}

	logger := pm.logger.With(
		zap.String("lookup_table_id", pool.LookupTableID.String()),
	)
	logger.Debug("Loading lookup table")

	// Загружаем состояние lookup table
	rpcClient := pm.client.GetRpcClient()
	lookupTable, err := addresslookuptable.GetAddressLookupTable(
		ctx,
		rpcClient,
		pool.LookupTableID,
	)
	if err != nil {
		return &PoolError{
			Stage:   "load_lookup_table",
			Message: "failed to load lookup table",
			Err:     err,
		}
	}

	// Сохраняем адреса
	pool.LookupTableAddresses = lookupTable.Addresses

	logger.Debug("Lookup table loaded successfully",
		zap.Int("addresses_count", len(pool.LookupTableAddresses)),
	)

	return nil
}

// Модифицируем существующий метод инициализации пула
func (pm *PoolManager) InitializePool(ctx context.Context, params *RaydiumPool) error {
	logger := pm.logger.With(
		zap.String("base_mint", params.BaseMint.String()),
		zap.String("quote_mint", params.QuoteMint.String()),
	)
	logger.Debug("Initializing new pool")

	if err := pm.validatePoolParameters(params); err != nil {
		return &PoolError{
			Stage:   "initialize",
			Message: "invalid pool parameters",
			Err:     err,
		}
	}

	// Добавляем загрузку lookup table после валидации параметров
	if err := pm.LoadPoolLookupTable(ctx, params); err != nil {
		return err
	}

	return nil
}

// PoolCalculator предоставляет методы для расчетов в пуле
type PoolCalculator struct {
	pool   *RaydiumPool
	state  *PoolState
	logger *zap.Logger // Добавляем logger в структуру
}

// NewPoolCalculator создает новый калькулятор для пула
func NewPoolCalculator(pool *RaydiumPool, state *PoolState, logger *zap.Logger) *PoolCalculator {
	return &PoolCalculator{
		pool:   pool,
		state:  state,
		logger: logger.Named("pool-calculator"), // Добавляем префикс для логгера
	}
}

// CalculateSwapAmount вычисляет количество токенов для свапа
func (pc *PoolCalculator) CalculateSwapAmount(
	amountIn uint64,
	slippageBps uint16,
	side SwapSide,
) (*SwapAmounts, error) {
	if amountIn == 0 {
		return nil, &PoolError{
			Stage:   "calculate_swap",
			Message: "amount in cannot be zero",
		}
	}

	// Конвертируем в big.Float для точных вычислений
	amountInF := new(big.Float).SetUint64(amountIn)
	baseReserveF := new(big.Float).SetUint64(pc.state.BaseReserve)
	quoteReserveF := new(big.Float).SetUint64(pc.state.QuoteReserve)

	// Вычисляем комиссию
	feeMultiplier := new(big.Float).SetFloat64(1 - float64(pc.pool.DefaultFeeBps)/10000)
	amountInAfterFee := new(big.Float).Mul(amountInF, feeMultiplier)

	var amountOut *big.Float
	if side == SwapSideIn {
		numerator := new(big.Float).Mul(amountInAfterFee, quoteReserveF)
		denominator := new(big.Float).Add(baseReserveF, amountInAfterFee)
		amountOut = new(big.Float).Quo(numerator, denominator)
	} else {
		numerator := new(big.Float).Mul(amountInAfterFee, baseReserveF)
		denominator := new(big.Float).Add(quoteReserveF, amountInAfterFee)
		amountOut = new(big.Float).Quo(numerator, denominator)
	}

	// Исправляем конвертацию в uint64
	amountOutU, _ := amountOut.Uint64()

	// Учитываем слиппаж для минимального выхода
	slippageMultiplier := new(big.Float).SetFloat64(1 - float64(slippageBps)/10000)
	minAmountOut := new(big.Float).Mul(new(big.Float).SetUint64(amountOutU), slippageMultiplier)

	// Исправляем конвертацию в uint64
	minAmountOutU, _ := minAmountOut.Uint64()

	return &SwapAmounts{
		AmountIn:     amountIn,
		AmountOut:    amountOutU,
		MinAmountOut: minAmountOutU,
		Fee:          pc.calculateFeeAmount(amountIn),
	}, nil
}

// SwapAmounts содержит результаты расчета свапа
type SwapAmounts struct {
	AmountIn     uint64
	AmountOut    uint64
	MinAmountOut uint64
	Fee          uint64
}

// PoolError представляет ошибку операций с пулом
type PoolError struct {
	Stage   string
	Message string
	Err     error
}

func (e *PoolError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("pool error at %s: %s: %v", e.Stage, e.Message, e.Err)
	}
	return fmt.Sprintf("pool error at %s: %s", e.Stage, e.Message)
}

// GetOptimalSwapAmount вычисляет оптимальное количество токенов для свапа
func (pc *PoolCalculator) GetOptimalSwapAmount(
	availableAmount uint64,
	targetAmount uint64,
	slippageBps uint16,
) (*SwapAmounts, error) {
	// Исправляем pm на pc.pool, так как мы находимся в методе PoolCalculator
	logger := pc.logger.With(
		zap.Uint64("available_amount", availableAmount),
		zap.Uint64("target_amount", targetAmount),
		zap.Uint16("slippage_bps", slippageBps),
	)
	logger.Debug("Calculating optimal swap amount")

	// Остальной код остается без изменений
	left := uint64(1)
	right := availableAmount
	var bestAmount *SwapAmounts

	for left <= right {
		mid := left + (right-left)/2

		amounts, err := pc.CalculateSwapAmount(mid, slippageBps, SwapSideIn)
		if err != nil {
			return nil, &PoolError{
				Stage:   "optimal_amount",
				Message: "failed to calculate swap amount",
				Err:     err,
			}
		}

		if amounts.AmountOut == targetAmount {
			return amounts, nil
		}

		if amounts.AmountOut < targetAmount {
			left = mid + 1
		} else {
			bestAmount = amounts
			right = mid - 1
		}
	}

	if bestAmount == nil {
		return nil, &PoolError{
			Stage:   "optimal_amount",
			Message: "could not find suitable amount",
		}
	}

	return bestAmount, nil
}

// validatePoolParameters проверяет параметры пула
func (pm *PoolManager) validatePoolParameters(pool *RaydiumPool) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	// Проверяем базовые параметры
	if pool.BaseMint.IsZero() || pool.QuoteMint.IsZero() {
		return fmt.Errorf("invalid mint addresses")
	}

	if pool.BaseDecimals == 0 || pool.QuoteDecimals == 0 {
		return fmt.Errorf("invalid decimals")
	}

	if pool.DefaultFeeBps == 0 || pool.DefaultFeeBps > 10000 {
		return fmt.Errorf("invalid fee bps")
	}

	// Проверяем параметры AMM
	if pool.AmmProgramID.IsZero() || pool.SerumProgramID.IsZero() {
		return fmt.Errorf("invalid program IDs")
	}

	// Проверяем параметры маркета
	if pool.MarketID.IsZero() || pool.MarketProgramID.IsZero() {
		return fmt.Errorf("invalid market parameters")
	}

	// Если указан lookup table ID, проверяем что он валидный
	if !pool.LookupTableID.IsZero() {
		// Проверка существования lookup table будет выполнена при загрузке
		logger := pm.logger.With(
			zap.String("lookup_table_id", pool.LookupTableID.String()),
		)
		logger.Debug("Pool has lookup table configuration")
	}
	return nil
}

// calculateFeeAmount вычисляет комиссию для заданной суммы
func (pc *PoolCalculator) calculateFeeAmount(amount uint64) uint64 {
	return amount * uint64(pc.pool.DefaultFeeBps) / 10000
}

// GetTokenAccounts получает или создает токен-аккаунты для пула
func (pm *PoolManager) GetTokenAccounts(
	ctx context.Context,
	owner solana.PublicKey,
	mint solana.PublicKey,
) (solana.PublicKey, error) {
	ata, _, err := solana.FindAssociatedTokenAddress(owner, mint)
	if err != nil {
		return solana.PublicKey{}, &PoolError{
			Stage:   "get_token_accounts",
			Message: "failed to find ATA",
			Err:     err,
		}
	}

	// Проверяем существование аккаунта
	account, err := pm.client.GetAccountInfo(ctx, ata)
	if err != nil {
		return solana.PublicKey{}, &PoolError{
			Stage:   "get_token_accounts",
			Message: "failed to get account info",
			Err:     err,
		}
	}

	// Если аккаунт не существует, возвращаем инструкцию для создания
	if account == nil || account.Value == nil {
		return ata, nil
	}

	return ata, nil
}

// GetMarketPrice получает текущую цену в пуле
func (pc *PoolCalculator) GetMarketPrice() float64 {
	if pc.state.BaseReserve == 0 || pc.state.QuoteReserve == 0 {
		return 0
	}

	baseF := float64(pc.state.BaseReserve)
	quoteF := float64(pc.state.QuoteReserve)

	baseDecimalAdj := math.Pow10(int(pc.pool.BaseDecimals))
	quoteDecimalAdj := math.Pow10(int(pc.pool.QuoteDecimals))

	return (quoteF / quoteDecimalAdj) / (baseF / baseDecimalAdj)
}

// Добавить в pool.go:
type RaydiumV5Pool struct {
	RaydiumPool
	PnlOwner    solana.PublicKey
	ModelDataId solana.PublicKey
	RecentRoot  *big.Int
	MaxOrders   uint64
	OrderStates []*big.Int
	TickSpacing uint16
}

// Методы для работы с v5 пулами
func (pm *PoolManager) InitializeV5Pool(ctx context.Context, params *RaydiumPoolV5) error {
	// TODO: implement
	return nil
}

// Методы для работы с LP токенами
func (pm *PoolManager) GetLPTokenBalance(ctx context.Context, owner solana.PublicKey) (uint64, error) {
	// TODO: implement
	return 0, nil
}

// Улучшить кэширование в pool.go:
type PoolCache struct {
	pool      *RaydiumPool
	state     *PoolState
	updatedAt time.Time
	ttl       time.Duration
}

// Добавить методы для анализа ликвидности:

func (p *PoolManager) GetLiquidityDepth(ctx context.Context, pool *RaydiumPool) (*LiquidityDepth, error) {
	// TODO: реализовать
	return nil, nil
}

// Реализовать concentrated liquidity:

type ConcentratedLiquidityPool struct {
	// TODO: определить структуру
}

func (p *PoolManager) InitializeConcentratedPool(ctx context.Context, params *ConcentratedPoolParams) error {
	// TODO: реализовать
	return nil
}
