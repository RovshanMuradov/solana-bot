// internal/dex/raydium/pool.go
package raydium

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"
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
	pool   *RaydiumV5Pool // Текущий активный пул
}

// NewPoolManager создает новый менеджер пула
func NewPoolManager(client blockchain.Client, logger *zap.Logger) *PoolManager {
	return &PoolManager{
		client: client,
		logger: logger.Named("pool-manager"),
		pool:   nil,
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

// RaydiumV5Pool представляет пул Raydium версии 5
type RaydiumV5Pool struct {
	RaydiumPool // Встраиваем базовую структуру пула

	// Основные параметры V5
	PnlOwner    solana.PublicKey // Владелец PnL (Profit and Loss)
	ModelDataId solana.PublicKey // ID модели данных пула
	RecentRoot  *big.Int         // Последний корневой хеш состояния пула
	MaxOrders   uint64           // Максимальное количество ордеров
	OrderStates []*big.Int       // Состояния ордеров
	TickSpacing uint16           // Шаг тиков цены

	// Дополнительные параметры V5
	LPMint       solana.PublicKey    // Минт LP токенов
	AdminKey     solana.PublicKey    // Ключ администратора пула
	ConfigParams V5ConfigParams      // Параметры конфигурации
	FeeAccounts  V5FeeAccounts       // Аккаунты для комиссий
	PoolState    V5PoolState         // Состояние пула
	PriceHistory []PriceHistoryPoint // История цен
}

// V5ConfigParams содержит параметры конфигурации пула V5
type V5ConfigParams struct {
	MinPriceRatio  *big.Int // Минимальное соотношение цен
	MaxPriceRatio  *big.Int // Максимальное соотношение цен
	MinBaseAmount  uint64   // Минимальное количество базового токена
	MinQuoteAmount uint64   // Минимальное количество котируемого токена
	MaxSlippageBps uint16   // Максимальный проскальзывание в базисных пунктах
	MaxLeverage    uint16   // Максимальное плечо
	ProtocolFee    uint16   // Комиссия протокола в базисных пунктах
	MinOrderSize   uint64   // Минимальный размер ордера
}

// V5FeeAccounts содержит аккаунты для комиссий
type V5FeeAccounts struct {
	ProtocolFeeAccount solana.PublicKey // Аккаунт для комиссий протокола
	TraderFeeAccount   solana.PublicKey // Аккаунт для комиссий трейдера
	LPFeeAccount       solana.PublicKey // Аккаунт для комиссий провайдеров ликвидности
}

// V5PoolState содержит текущее состояние пула
type V5PoolState struct {
	BaseReserve     uint64     // Резерв базового токена
	QuoteReserve    uint64     // Резерв котируемого токена
	LPSupply        uint64     // Общее предложение LP токенов
	LastUpdateSlot  uint64     // Слот последнего обновления
	SwapEnabled     bool       // Включен ли свап
	PriceMultiplier *big.Int   // Мультипликатор цены
	CurrentPrice    *big.Float // Текущая цена
	TVL             *big.Float // Total Value Locked
}

// PriceHistoryPoint представляет точку в истории цен
type PriceHistoryPoint struct {
	Timestamp time.Time  // Временная метка
	Price     *big.Float // Цена
	Volume    uint64     // Объем
}

// Методы для работы с V5Pool

// NewRaydiumV5Pool создает новый экземпляр V5 пула
func NewRaydiumV5Pool(baseParams *RaydiumPool) *RaydiumV5Pool {
	return &RaydiumV5Pool{
		RaydiumPool: *baseParams,
		ConfigParams: V5ConfigParams{
			MinPriceRatio: new(big.Int),
			MaxPriceRatio: new(big.Int),
		},
		PoolState: V5PoolState{
			PriceMultiplier: new(big.Int),
			CurrentPrice:    new(big.Float),
			TVL:             new(big.Float),
		},
		PriceHistory: make([]PriceHistoryPoint, 0),
	}
}

// GetCurrentState возвращает текущее состояние пула
func (p *RaydiumV5Pool) GetCurrentState() V5PoolState {
	return p.PoolState
}

// UpdateState обновляет состояние пула
func (p *RaydiumV5Pool) UpdateState(newState V5PoolState) {
	p.PoolState = newState
	// Добавляем точку в историю цен
	p.PriceHistory = append(p.PriceHistory, PriceHistoryPoint{
		Timestamp: time.Now(),
		Price:     newState.CurrentPrice,
		Volume:    0, // Нужно добавить расчет объема
	})
}

// GetFeeAccounts возвращает аккаунты для комиссий
func (p *RaydiumV5Pool) GetFeeAccounts() V5FeeAccounts {
	return p.FeeAccounts
}

// IsSwapEnabled проверяет, включен ли свап в пуле
func (p *RaydiumV5Pool) IsSwapEnabled() bool {
	return p.PoolState.SwapEnabled
}

// GetTVL возвращает Total Value Locked
func (p *RaydiumV5Pool) GetTVL() *big.Float {
	return p.PoolState.TVL
}

// Вспомогательный метод для валидации model data
func (pm *PoolManager) validateModelData(ctx context.Context, modelDataId solana.PublicKey) error {
	if modelDataId.IsZero() {
		return fmt.Errorf("model data ID cannot be zero")
	}

	// Проверяем существование аккаунта
	account, err := pm.client.GetAccountInfo(ctx, modelDataId)
	if err != nil {
		return fmt.Errorf("failed to get model data account: %w", err)
	}

	if account == nil || account.Value == nil {
		return fmt.Errorf("model data account not found")
	}

	// Здесь можно добавить дополнительную валидацию данных модели
	// в зависимости от требований

	return nil
}

// SetPool устанавливает текущий активный пул
func (pm *PoolManager) SetPool(pool *RaydiumV5Pool) {
	pm.pool = pool
}

// GetPool возвращает текущий активный пул
func (pm *PoolManager) GetPool() *RaydiumV5Pool {
	return pm.pool
}

// InitializeV5Pool инициализирует пул версии 5 Raydium
func (pm *PoolManager) InitializeV5Pool(ctx context.Context, params *RaydiumV5Pool) error {
	logger := pm.logger.With(
		zap.String("base_mint", params.BaseMint.String()),
		zap.String("quote_mint", params.QuoteMint.String()),
		zap.String("pnl_owner", params.PnlOwner.String()),
	)
	logger.Debug("Initializing new V5 pool")

	// Проверяем базовые параметры пула
	if err := pm.validatePoolParameters(&params.RaydiumPool); err != nil {
		return &PoolError{
			Stage:   "initialize_v5",
			Message: "invalid pool parameters",
			Err:     err,
		}
	}

	// Дополнительная валидация параметров V5
	if err := pm.validateV5Parameters(params); err != nil {
		return &PoolError{
			Stage:   "initialize_v5",
			Message: "invalid v5 specific parameters",
			Err:     err,
		}
	}

	// Загружаем lookup table если она указана
	if err := pm.LoadPoolLookupTable(ctx, &params.RaydiumPool); err != nil {
		return err
	}

	// Проверяем наличие и валидность model data account
	if err := pm.validateModelData(ctx, params.ModelDataId); err != nil {
		return &PoolError{
			Stage:   "initialize_v5",
			Message: "invalid model data account",
			Err:     err,
		}
	}

	return nil
}

// validateV5Parameters проверяет специфичные для V5 параметры
func (pm *PoolManager) validateV5Parameters(pool *RaydiumV5Pool) error {
	if pool.PnlOwner.IsZero() {
		return fmt.Errorf("pnl owner cannot be zero")
	}

	if pool.ModelDataId.IsZero() {
		return fmt.Errorf("model data id cannot be zero")
	}

	if pool.MaxOrders == 0 {
		return fmt.Errorf("max orders must be greater than zero")
	}

	if pool.TickSpacing == 0 {
		return fmt.Errorf("tick spacing must be greater than zero")
	}

	return nil
}

// UiTokenAmount представляет количество токенов с учетом десятичных знаков
type UiTokenAmount struct {
	// Точное количество токенов в виде строки для предотвращения потери точности
	Amount string `json:"amount"`

	// Количество десятичных знаков токена
	Decimals uint8 `json:"decimals"`

	// Форматированное количество токенов с учетом десятичных знаков
	UiAmount float64 `json:"uiAmount"`

	// Форматированное количество в виде строки
	UiAmountString string `json:"uiAmountString"`
}

// NewUiTokenAmount создает новый UiTokenAmount
func NewUiTokenAmount(amount uint64, decimals uint8) *UiTokenAmount {
	// Конвертируем amount в float64 с учетом decimals
	uiAmount := float64(amount) / math.Pow10(int(decimals))

	return &UiTokenAmount{
		Amount:         strconv.FormatUint(amount, 10),
		Decimals:       decimals,
		UiAmount:       uiAmount,
		UiAmountString: fmt.Sprintf("%f", uiAmount),
	}
}

// ToUint64 конвертирует UiAmount обратно в uint64
func (u *UiTokenAmount) ToUint64() (uint64, error) {
	return strconv.ParseUint(u.Amount, 10, 64)
}

// String возвращает строковое представление количества токенов
func (u *UiTokenAmount) String() string {
	return u.UiAmountString
}

// FromDecimals создает UiTokenAmount из количества и decimals
func FromDecimals(amount uint64, decimals uint8) *UiTokenAmount {
	return NewUiTokenAmount(amount, decimals)
}

// Parse парсит значение из JSON
func (u *UiTokenAmount) UnmarshalJSON(data []byte) error {
	// Временная структура для парсинга
	type Alias UiTokenAmount
	aux := &struct {
		Amount         string  `json:"amount"`
		Decimals       uint8   `json:"decimals"`
		UiAmount       float64 `json:"uiAmount"`
		UiAmountString string  `json:"uiAmountString"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Просто присваиваем строковые значения
	u.Amount = aux.Amount
	u.Decimals = aux.Decimals
	u.UiAmount = aux.UiAmount
	u.UiAmountString = aux.UiAmountString

	return nil
}

// MarshalJSON конвертирует в JSON
func (u UiTokenAmount) MarshalJSON() ([]byte, error) {
	// Временная структура для маршалинга
	return json.Marshal(&struct {
		Amount         string  `json:"amount"`
		Decimals       uint8   `json:"decimals"`
		UiAmount       float64 `json:"uiAmount"`
		UiAmountString string  `json:"uiAmountString"`
	}{
		Amount:         u.Amount,
		Decimals:       u.Decimals,
		UiAmount:       u.UiAmount,
		UiAmountString: u.UiAmountString,
	})
}

// Методы для работы с LP токенами
// GetLPTokenBalance получает баланс LP токенов для указанного владельца
func (pm *PoolManager) GetLPTokenBalance(ctx context.Context, owner solana.PublicKey) (uint64, error) {
	if pm.pool == nil {
		return 0, &PoolError{
			Stage:   "get_lp_balance",
			Message: "no active pool set",
		}
	}

	logger := pm.logger.With(
		zap.String("owner", owner.String()),
		zap.String("lp_mint", pm.pool.LPMint.String()),
	)
	logger.Debug("Getting LP token balance")

	// Получаем associated token address для LP токенов
	lpTokenATA, _, err := solana.FindAssociatedTokenAddress(owner, pm.pool.LPMint)
	if err != nil {
		return 0, &PoolError{
			Stage:   "get_lp_balance",
			Message: "failed to find LP token ATA",
			Err:     err,
		}
	}

	balance, err := pm.client.GetTokenAccountBalance(
		ctx,
		lpTokenATA,
		rpc.CommitmentConfirmed,
	)
	if err != nil {
		return 0, &PoolError{
			Stage:   "get_lp_balance",
			Message: "failed to get token balance",
			Err:     err,
		}
	}

	if balance == nil || balance.Value == nil {
		return 0, nil
	}

	// Конвертируем строковое значение в uint64
	amount, err := strconv.ParseUint(balance.Value.Amount, 10, 64)
	if err != nil {
		return 0, &PoolError{
			Stage:   "get_lp_balance",
			Message: "failed to parse token amount",
			Err:     err,
		}
	}

	return amount, nil
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
