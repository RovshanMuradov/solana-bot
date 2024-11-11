// Создадим отдельный пакет для работы с API
// internal/dex/raydium/pool_api.go
package raydium

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
