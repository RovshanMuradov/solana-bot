// Создадим отдельный пакет для работы с API
// internal/dex/raydium/pool_api.go
package raydium

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

const (
	apiBaseURL            = "https://api-sm.yvesdev.com/api"
	defaultRequestTimeout = 5 * time.Second
	maxRetries            = 3
)

// APIService представляет сервис для работы с API
type APIService struct {
	client  *http.Client
	logger  *zap.Logger
	baseURL string
}

// APIPoolResponse представляет структуру ответа от API
type APIPoolResponse struct {
	Success bool      `json:"success"`
	Pool    []APIPool `json:"pool"`
}

// APIPool представляет структуру данных пула из API
type APIPool struct {
	ID          string `json:"poolID"`
	MarketID    string `json:"marketID"`
	LPMint      string `json:"lpMint"`
	Creator     string `json:"creator,omitempty"`
	TokenSymbol string `json:"tokenSymbol"`
	TokenName   string `json:"tokenName"`
	OpenTimeMs  string `json:"openTimeMs"`
	Timestamp   string `json:"timestamp"`
	Image       string `json:"image,omitempty"`
}

// NewAPIService создает новый экземпляр API сервиса
func NewAPIService(logger *zap.Logger) *APIService {
	return &APIService{
		client: &http.Client{
			Timeout: defaultRequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxConnsPerHost:     100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger:  logger.Named("api-service"),
		baseURL: apiBaseURL,
	}
}

// GetPoolByToken получает информацию о пуле по токену с retry механизмом
func (s *APIService) GetPoolByToken(ctx context.Context, tokenMint solana.PublicKey) (*Pool, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		pool, err := s.fetchPoolByToken(ctx, tokenMint)
		if err == nil {
			return pool, nil
		}
		lastErr = err

		// Если контекст отменен, прекращаем попытки
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		s.logger.Warn("retry getting pool",
			zap.String("token", tokenMint.String()),
			zap.Int("attempt", attempt+1),
			zap.Error(err))

		// Экспоненциальная задержка между попытками
		if attempt < maxRetries {
			time.Sleep(time.Second * time.Duration(1<<uint(attempt)))
		}
	}
	return nil, fmt.Errorf("failed to get pool after %d attempts: %w", maxRetries+1, lastErr)
}

// fetchPoolByToken выполняет один запрос к API
func (s *APIService) fetchPoolByToken(ctx context.Context, tokenMint solana.PublicKey) (*Pool, error) {
	url := fmt.Sprintf("%s/getBlockByToken?token=%s", s.baseURL, tokenMint.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	start := time.Now()
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	s.logger.Debug("api request completed",
		zap.Duration("duration", duration),
		zap.String("token", tokenMint.String()),
		zap.Int("status", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response APIPoolResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if err := s.validateAPIResponse(&response, tokenMint); err != nil {
		return nil, fmt.Errorf("invalid api response: %w", err)
	}

	// Конвертируем ответ API в структуру Pool
	pool, err := s.convertAPIResponseToPool(response.Pool[0])
	if err != nil {
		return nil, fmt.Errorf("failed to convert api response: %w", err)
	}

	s.logger.Debug("pool found",
		zap.String("pool_id", pool.ID.String()),
		zap.String("market_id", pool.MarketID.String()),
		zap.String("token_symbol", pool.TokenSymbol),
		zap.String("token_name", pool.TokenName))

	return pool, nil
}

// validateAPIResponse проверяет корректность ответа API
func (s *APIService) validateAPIResponse(response *APIPoolResponse, expectedToken solana.PublicKey) error {
	if !response.Success {
		return fmt.Errorf("api returned unsuccessful response")
	}

	if len(response.Pool) == 0 {
		return fmt.Errorf("no pool found for token %s", expectedToken.String())
	}

	apiPool := response.Pool[0]
	if apiPool.ID == "" || apiPool.MarketID == "" || apiPool.LPMint == "" {
		return fmt.Errorf("required pool fields are missing")
	}

	// Проверяем, что LPMint соответствует ожидаемому токену или является его пулом
	lpMint, err := solana.PublicKeyFromBase58(apiPool.LPMint)
	if err != nil {
		return fmt.Errorf("invalid lp mint address: %w", err)
	}

	if !lpMint.Equals(expectedToken) {
		s.logger.Warn("lpMint does not match expected token",
			zap.String("expected", expectedToken.String()),
			zap.String("got", lpMint.String()))
	}

	return nil
}

// convertAPIResponseToPool конвертирует ответ API в структуру Pool
func (s *APIService) convertAPIResponseToPool(apiPool APIPool) (*Pool, error) {
	// Конвертируем все адреса с проверкой ошибок
	poolID, err := solana.PublicKeyFromBase58(apiPool.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid pool ID: %w", err)
	}

	marketID, err := solana.PublicKeyFromBase58(apiPool.MarketID)
	if err != nil {
		return nil, fmt.Errorf("invalid market ID: %w", err)
	}

	lpMint, err := solana.PublicKeyFromBase58(apiPool.LPMint)
	if err != nil {
		return nil, fmt.Errorf("invalid LP mint: %w", err)
	}

	// Парсим временную метку
	openTime, err := strconv.ParseInt(apiPool.OpenTimeMs, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid open time: %w", err)
	}

	timestamp, err := time.Parse(time.RFC3339, apiPool.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	var creator solana.PublicKey
	if apiPool.Creator != "" {
		creator, err = solana.PublicKeyFromBase58(apiPool.Creator)
		if err != nil {
			return nil, fmt.Errorf("invalid creator address: %w", err)
		}
	}

	pool := &Pool{
		ID:          poolID,
		MarketID:    marketID,
		LPMint:      lpMint,
		Creator:     creator,
		TokenSymbol: apiPool.TokenSymbol,
		TokenName:   apiPool.TokenName,
		OpenTimeMs:  openTime,
		Timestamp:   timestamp,
		IsFromAPI:   true,
		Version:     PoolVersionV4, // Устанавливаем версию V4 по умолчанию
	}

	if err := s.validateConvertedPool(pool); err != nil {
		return nil, fmt.Errorf("pool validation failed: %w", err)
	}

	return pool, nil
}

// validateConvertedPool проверяет валидность сконвертированного пула
func (s *APIService) validateConvertedPool(pool *Pool) error {
	if pool.ID.IsZero() {
		return fmt.Errorf("zero pool ID")
	}
	if pool.MarketID.IsZero() {
		return fmt.Errorf("zero market ID")
	}
	if pool.LPMint.IsZero() {
		return fmt.Errorf("zero LP mint")
	}
	if pool.TokenSymbol == "" {
		return fmt.Errorf("empty token symbol")
	}
	if pool.TokenName == "" {
		return fmt.Errorf("empty token name")
	}
	if pool.OpenTimeMs <= 0 {
		return fmt.Errorf("invalid open time")
	}
	if pool.Timestamp.IsZero() {
		return fmt.Errorf("zero timestamp")
	}

	return nil
}

// Добавить методы:

// GetPools получает все доступные пулы для пары токенов
func (s *APIService) GetPools(ctx context.Context, baseToken, quoteToken solana.PublicKey) ([]*Pool, error) {
	s.logger.Debug("fetching pools for token pair",
		zap.String("base_token", baseToken.String()),
		zap.String("quote_token", quoteToken.String()))

	// 1. Делаем запросы к API для обоих токенов параллельно
	type poolResult struct {
		pools []*Pool
		err   error
	}

	baseTokenChan := make(chan poolResult, 1)
	quoteTokenChan := make(chan poolResult, 1)

	// Запускаем горутины для параллельных запросов
	go func() {
		pools, err := s.fetchPoolsByToken(ctx, baseToken)
		baseTokenChan <- poolResult{pools: pools, err: err}
	}()

	go func() {
		pools, err := s.fetchPoolsByToken(ctx, quoteToken)
		quoteTokenChan <- poolResult{pools: pools, err: err}
	}()

	// 2. Собираем результаты
	var allPools []*Pool
	seenPools := make(map[string]bool)

	// Получаем результаты с таймаутом
	timeout := time.After(s.client.Timeout)

	// Обрабатываем результат базового токена
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timeout:
		return nil, fmt.Errorf("timeout while fetching base token pools")
	case result := <-baseTokenChan:
		if result.err == nil {
			for _, pool := range result.pools {
				if !seenPools[pool.ID.String()] {
					allPools = append(allPools, pool)
					seenPools[pool.ID.String()] = true
				}
			}
		} else {
			s.logger.Warn("failed to fetch base token pools", zap.Error(result.err))
		}
	}

	// Обрабатываем результат котируемого токена
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timeout:
		return nil, fmt.Errorf("timeout while fetching quote token pools")
	case result := <-quoteTokenChan:
		if result.err == nil {
			for _, pool := range result.pools {
				if !seenPools[pool.ID.String()] {
					allPools = append(allPools, pool)
					seenPools[pool.ID.String()] = true
				}
			}
		} else {
			s.logger.Warn("failed to fetch quote token pools", zap.Error(result.err))
		}
	}

	if len(allPools) == 0 {
		return nil, fmt.Errorf("no pools found for tokens %s and %s", baseToken, quoteToken)
	}

	// 3. Фильтруем пулы, где есть оба токена
	var validPools []*Pool
	for _, pool := range allPools {
		if err := s.ValidatePool(ctx, pool, baseToken, quoteToken); err == nil {
			validPools = append(validPools, pool)
		}
	}

	// 4. Сортируем по ликвидности (от большей к меньшей)
	sort.Slice(validPools, func(i, j int) bool {
		liquidityI := validPools[i].State.BaseReserve + validPools[i].State.QuoteReserve
		liquidityJ := validPools[j].State.BaseReserve + validPools[j].State.QuoteReserve
		return liquidityI > liquidityJ
	})

	s.logger.Debug("pools retrieved successfully",
		zap.Int("total_pools", len(allPools)),
		zap.Int("valid_pools", len(validPools)))

	return validPools, nil
}

// ValidatePool проверяет валидность пула для свапа
func (s *APIService) ValidatePool(ctx context.Context, pool *Pool, baseToken, quoteToken solana.PublicKey) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	// 1. Проверяем наличие обоих токенов в пуле
	hasBaseToken := pool.BaseMint.Equals(baseToken) || pool.QuoteMint.Equals(baseToken)
	hasQuoteToken := pool.BaseMint.Equals(quoteToken) || pool.QuoteMint.Equals(quoteToken)

	if !hasBaseToken || !hasQuoteToken {
		return fmt.Errorf("pool %s does not contain both required tokens", pool.ID)
	}

	// 2. Проверяем статус пула
	if pool.State.Status != PoolStatusActive {
		return fmt.Errorf("pool %s is not active (status: %d)", pool.ID, pool.State.Status)
	}

	// 3. Проверяем ликвидность
	minLiquidity := uint64(1000000) // Минимальная ликвидность в lamports
	if pool.State.BaseReserve < minLiquidity || pool.State.QuoteReserve < minLiquidity {
		return fmt.Errorf("pool %s has insufficient liquidity (base: %d, quote: %d)",
			pool.ID,
			pool.State.BaseReserve,
			pool.State.QuoteReserve)
	}

	// 4. Проверяем версию пула
	if !pool.Version.IsValid() {
		return fmt.Errorf("pool %s has unsupported version: %d", pool.ID, pool.Version)
	}

	// Дополнительные проверки
	if pool.DefaultFeeBps > 10000 { // 100% в базисных пунктах
		return fmt.Errorf("pool %s has invalid fee: %d bps", pool.ID, pool.DefaultFeeBps)
	}

	if pool.BaseDecimals == 0 || pool.QuoteDecimals == 0 {
		return fmt.Errorf("pool %s has invalid decimals (base: %d, quote: %d)",
			pool.ID,
			pool.BaseDecimals,
			pool.QuoteDecimals)
	}

	s.logger.Debug("pool validated successfully",
		zap.String("pool_id", pool.ID.String()),
		zap.Uint64("base_reserve", pool.State.BaseReserve),
		zap.Uint64("quote_reserve", pool.State.QuoteReserve),
		zap.Uint16("fee_bps", pool.DefaultFeeBps))

	return nil
}

// fetchPoolsByToken вспомогательный метод для получения пулов по токену
func (s *APIService) fetchPoolsByToken(ctx context.Context, token solana.PublicKey) ([]*Pool, error) {
	url := fmt.Sprintf("%s/getPoolsByToken?token=%s", s.baseURL, token.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Success bool      `json:"success"`
		Pools   []APIPool `json:"pools"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("API returned unsuccessful response")
	}

	pools := make([]*Pool, 0, len(response.Pools))
	for _, apiPool := range response.Pools {
		pool, err := s.convertAPIResponseToPool(apiPool)
		if err != nil {
			s.logger.Warn("failed to convert pool",
				zap.String("pool_id", apiPool.ID),
				zap.Error(err))
			continue
		}
		pools = append(pools, pool)
	}

	return pools, nil
}

// GetPoolByPair - ключевой метод для поиска пула по паре токенов
func (s *APIService) GetPoolByPair(ctx context.Context, tokenA, tokenB solana.PublicKey) (*Pool, error) {
	s.logger.Debug("searching pool for token pair",
		zap.String("token_a", tokenA.String()),
		zap.String("token_b", tokenB.String()))

	// 1. Делаем запросы к API для обоих токенов
	type poolResult struct {
		pools []*Pool
		err   error
	}

	results := make(chan poolResult, 2)

	// Запускаем параллельные запросы
	for _, token := range []solana.PublicKey{tokenA, tokenB} {
		go func(t solana.PublicKey) {
			url := fmt.Sprintf("%s/getBlockByToken?token=%s", s.baseURL, t.String())

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				results <- poolResult{err: fmt.Errorf("create request: %w", err)}
				return
			}

			resp, err := s.client.Do(req)
			if err != nil {
				results <- poolResult{err: fmt.Errorf("execute request: %w", err)}
				return
			}
			defer resp.Body.Close()

			var response APIPoolResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				results <- poolResult{err: fmt.Errorf("decode response: %w", err)}
				return
			}

			pools := make([]*Pool, 0, len(response.Pool))
			for _, apiPool := range response.Pool {
				pool, err := s.convertAPIResponseToPool(apiPool)
				if err != nil {
					s.logger.Warn("failed to convert pool",
						zap.String("pool_id", apiPool.ID),
						zap.Error(err))
					continue
				}
				pools = append(pools, pool)
			}

			results <- poolResult{pools: pools}
		}(token)
	}

	// 2. Собираем результаты
	var allPools []*Pool
	seenPools := make(map[string]bool)

	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-results:
			if result.err != nil {
				s.logger.Warn("failed to fetch pools", zap.Error(result.err))
				continue
			}
			for _, pool := range result.pools {
				if !seenPools[pool.ID.String()] {
					allPools = append(allPools, pool)
					seenPools[pool.ID.String()] = true
				}
			}
		}
	}

	// 3. Ищем подходящий пул
	var bestPool *Pool
	var maxLiquidity uint64

	for _, pool := range allPools {
		// Проверяем наличие обоих токенов
		hasTokenA := pool.BaseMint.Equals(tokenA) || pool.QuoteMint.Equals(tokenA)
		hasTokenB := pool.BaseMint.Equals(tokenB) || pool.QuoteMint.Equals(tokenB)

		if !hasTokenA || !hasTokenB {
			continue
		}

		// Проверяем жизнеспособность пула
		if err := s.IsPoolViable(ctx, pool); err != nil {
			s.logger.Debug("pool not viable",
				zap.String("pool_id", pool.ID.String()),
				zap.Error(err))
			continue
		}

		// Оцениваем ликвидность
		totalLiquidity := pool.State.BaseReserve + pool.State.QuoteReserve
		if totalLiquidity > maxLiquidity {
			maxLiquidity = totalLiquidity
			bestPool = pool
		}
	}

	if bestPool == nil {
		return nil, fmt.Errorf("no viable pools found for tokens %s and %s", tokenA, tokenB)
	}

	s.logger.Info("found best pool",
		zap.String("pool_id", bestPool.ID.String()),
		zap.String("market_id", bestPool.MarketID.String()),
		zap.Uint64("base_reserve", bestPool.State.BaseReserve),
		zap.Uint64("quote_reserve", bestPool.State.QuoteReserve))

	return bestPool, nil
}

// IsPoolViable - проверка жизнеспособности пула
func (s *APIService) IsPoolViable(ctx context.Context, pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	// 1. Проверка статуса пула
	if pool.State.Status != PoolStatusActive {
		return fmt.Errorf("pool is not active (status: %d)", pool.State.Status)
	}

	// 2. Проверка возраста пула
	poolAge := time.Since(pool.Timestamp)
	minPoolAge := 24 * time.Hour // минимальный возраст пула
	if poolAge < minPoolAge {
		return fmt.Errorf("pool is too young (age: %v, min required: %v)", poolAge, minPoolAge)
	}

	// 3. Проверка объема торгов
	const minLiquidity = uint64(1000000) // минимальная ликвидность в lamports
	if pool.State.BaseReserve < minLiquidity || pool.State.QuoteReserve < minLiquidity {
		return fmt.Errorf("insufficient liquidity (base: %d, quote: %d)",
			pool.State.BaseReserve,
			pool.State.QuoteReserve)
	}

	// 4. Проверка глубины пула
	// Рассчитываем соотношение резервов
	baseToQuoteRatio := float64(pool.State.BaseReserve) / float64(pool.State.QuoteReserve)
	const maxRatioDeviation = 10.0 // максимальное допустимое отклонение от 1:1

	if baseToQuoteRatio > maxRatioDeviation || baseToQuoteRatio < 1/maxRatioDeviation {
		return fmt.Errorf("unbalanced reserves ratio: %.2f", baseToQuoteRatio)
	}

	// 5. Проверка спреда
	// Для пулов Raydium спред определяется комиссией
	const maxFeeBps = 1000 // максимально допустимая комиссия (10%)
	if pool.DefaultFeeBps > maxFeeBps {
		return fmt.Errorf("fee too high: %d bps", pool.DefaultFeeBps)
	}

	// Дополнительные проверки
	if !pool.Version.IsValid() {
		return fmt.Errorf("unsupported pool version: %d", pool.Version)
	}

	if time.Now().Before(time.UnixMilli(pool.OpenTimeMs)) {
		return fmt.Errorf("pool not yet opened")
	}

	s.logger.Debug("pool viability check passed",
		zap.String("pool_id", pool.ID.String()),
		zap.Duration("age", poolAge),
		zap.Float64("reserves_ratio", baseToQuoteRatio),
		zap.Uint16("fee_bps", pool.DefaultFeeBps))

	return nil
}
