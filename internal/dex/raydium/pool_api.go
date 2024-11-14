// internal/dex/raydium/pool_api.go
package raydium

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// Константа для WSOL
const WSOL_ADDRESS = "So11111111111111111111111111111111111111112"

// APIService представляет сервис для работы с API
type APIService struct {
	dsClient *Service
	logger   *zap.Logger
}

// NewAPIService создает новый экземпляр API сервиса
func NewAPIService(logger *zap.Logger) *APIService {
	return &APIService{
		dsClient: NewService(logger),
		logger:   logger.Named("api-service"),
	}
}

// GetPoolByToken получает информацию о пуле по токену
func (s *APIService) GetPoolByToken(ctx context.Context, tokenMint solana.PublicKey) (*Pool, error) {
	// Получаем пару с WSOL через DexScreener
	pairInfo, err := s.dsClient.GetPoolByToken(ctx, tokenMint, WSOL_ADDRESS)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool from DexScreener: %w", err)
	}

	return s.convertDexScreenerPairToPool(pairInfo)
}

// ValidatePool проверяет валидность пула для свапа
func (s *APIService) ValidatePool(ctx context.Context, pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	// Получаем актуальную информацию о пуле
	pairInfo, err := s.dsClient.MonitorPair(ctx, pool.ID.String())
	if err != nil {
		return fmt.Errorf("failed to get pair info: %w", err)
	}

	// Проверка DEX
	if pairInfo.DexId != raydiumDex {
		return fmt.Errorf("invalid DEX: expected Raydium, got %s", pairInfo.DexId)
	}

	// Проверка сети
	if pairInfo.ChainId != solanaChain {
		return fmt.Errorf("invalid chain: expected Solana, got %s", pairInfo.ChainId)
	}

	// Проверка наличия WSOL
	if pairInfo.BaseToken.Address != WSOL_ADDRESS && pairInfo.QuoteToken.Address != WSOL_ADDRESS {
		return fmt.Errorf("pool does not contain WSOL token")
	}

	// Проверка минимальной ликвидности (например, $10k)
	const minLiquidityUSD = 10000.0
	if pairInfo.Liquidity.USD < minLiquidityUSD {
		return fmt.Errorf("insufficient liquidity: $%.2f (minimum $%.2f required)",
			pairInfo.Liquidity.USD, minLiquidityUSD)
	}

	// Проверка возраста пула (минимум 24 часа)
	const minPoolAge = 24 * time.Hour
	poolAge := time.Since(time.Unix(pairInfo.PairCreatedAt/1000, 0))
	if poolAge < minPoolAge {
		return fmt.Errorf("pool too young: age %v (minimum %v required)", poolAge, minPoolAge)
	}

	// Проверка наличия цены
	if pairInfo.PriceNative == "" {
		return fmt.Errorf("no price data available")
	}

	// Проверка корректности адреса пула
	if pairInfo.PairAddress == "" || pairInfo.PairAddress != pool.ID.String() {
		return fmt.Errorf("invalid pool address")
	}

	s.logger.Debug("pool validation passed",
		zap.String("pool_id", pool.ID.String()),
		zap.String("dex", pairInfo.DexId),
		zap.Float64("liquidity_usd", pairInfo.Liquidity.USD),
		zap.Duration("age", poolAge))

	return nil
}

// StartPriceMonitoring запускает мониторинг цены
func (s *APIService) StartPriceMonitoring(ctx context.Context, pool *Pool) (*PriceMonitor, error) {
	// Получаем начальную цену
	pairInfo, err := s.dsClient.MonitorPair(ctx, pool.ID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get initial price: %w", err)
	}

	initialPrice, err := getInitialPrice(pairInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get initial price: %w", err)
	}

	return s.dsClient.StartPriceMonitoring(ctx, pool.ID.String(), initialPrice)
}

// Вспомогательные методы

func (s *APIService) convertDexScreenerPairToPool(pair *PairInfo) (*Pool, error) {
	if pair == nil {
		return nil, fmt.Errorf("pair info cannot be nil")
	}

	poolID, err := solana.PublicKeyFromBase58(pair.PairAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid pool address: %w", err)
	}

	baseMint, err := solana.PublicKeyFromBase58(pair.BaseToken.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid base token address: %w", err)
	}

	quoteMint, err := solana.PublicKeyFromBase58(pair.QuoteToken.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid quote token address: %w", err)
	}

	// Определяем, является ли WSOL базовым или котируемым токеном
	isWsolBase := pair.BaseToken.Address == WSOL_ADDRESS

	pool := &Pool{
		ID:          poolID,
		BaseMint:    baseMint,
		QuoteMint:   quoteMint,
		TokenSymbol: pair.BaseToken.Symbol,
		Version:     PoolVersionV4,
		State: PoolState{
			BaseReserve:  uint64(pair.Liquidity.Base),
			QuoteReserve: uint64(pair.Liquidity.Quote),
			Status:       PoolStatusActive,
		},
		DefaultFeeBps: 30, // Стандартная комиссия Raydium
		IsFromAPI:     true,
	}

	// Если WSOL - котируемый токен, меняем местами базовый и котируемый токены
	if !isWsolBase {
		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.State.BaseReserve, pool.State.QuoteReserve = pool.State.QuoteReserve, pool.State.BaseReserve
	}

	return pool, nil
}

func getInitialPrice(pair *PairInfo) (float64, error) {
	if pair.BaseToken.Address == WSOL_ADDRESS {
		return 1.0, nil // Если WSOL базовый, цена уже в SOL
	}
	// Если WSOL котируемый, инвертируем цену
	return 1.0 / float64(pair.Liquidity.Quote), nil
}
