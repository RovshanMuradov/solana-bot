// Создадим отдельный пакет для работы с API
// internal/dex/raydium/api.go
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

const apiBaseURL = "https://api-sm.yvesdev.com/api"

// APIService представляет сервис для работы с API
type APIService struct {
	client  *http.Client
	logger  *zap.Logger
	baseURL string
}

// PoolResponse представляет структуру ответа от API
type PoolResponse struct {
	Success bool `json:"success"`
	Pool    []struct {
		ID          string `json:"poolID"`
		MarketID    string `json:"marketID"`
		LPMint      string `json:"lpMint"`
		Creator     string `json:"creator"`
		TokenSymbol string `json:"tokenSymbol"`
		TokenName   string `json:"tokenName"`
		OpenTimeMs  string `json:"openTimeMs"`
		Timestamp   string `json:"timestamp"`
	} `json:"pool"`
}

// NewAPIService создает новый экземпляр API сервиса
func NewAPIService(logger *zap.Logger) *APIService {
	return &APIService{
		client: &http.Client{
			Timeout: 5 * time.Second,
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

// GetPoolByToken получает информацию о пуле по токену
// Обновим метод GetPoolByToken для использования нового конвертера
func (s *APIService) GetPoolByToken(ctx context.Context, tokenMint solana.PublicKey) (*Pool, error) {
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

	s.logger.Debug("api request completed",
		zap.Duration("duration", time.Since(start)),
		zap.String("token", tokenMint.String()),
		zap.Int("status", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response PoolResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !response.Success || len(response.Pool) == 0 {
		return nil, fmt.Errorf("no pool found for token %s", tokenMint.String())
	}

	// Используем новый конвертер
	pool, err := s.convertAPIDataToPool(response.Pool[0])
	if err != nil {
		return nil, fmt.Errorf("failed to convert API data: %w", err)
	}

	return pool, nil
}

// IsPoolValid проверяет валидность пула
func (s *APIService) IsPoolValid(pool *Pool) bool {
	if pool == nil {
		return false
	}

	// Проверяем что все необходимые PublicKey поля не нулевые
	if pool.ID.IsZero() ||
		pool.MarketID.IsZero() ||
		pool.LPMint.IsZero() ||
		pool.Creator.IsZero() {
		return false
	}

	// Проверяем наличие обязательных строковых полей
	if pool.TokenSymbol == "" || pool.TokenName == "" {
		return false
	}

	// Проверяем временные метки
	if pool.OpenTimeMs <= 0 {
		return false
	}

	// Проверяем что временная метка не нулевая
	if pool.Timestamp.IsZero() {
		return false
	}

	return true
}

// Также добавим метод для безопасной конвертации данных API
func (s *APIService) convertAPIDataToPool(apiData struct {
	ID          string `json:"poolID"`
	MarketID    string `json:"marketID"`
	LPMint      string `json:"lpMint"`
	Creator     string `json:"creator"`
	TokenSymbol string `json:"tokenSymbol"`
	TokenName   string `json:"tokenName"`
	OpenTimeMs  string `json:"openTimeMs"`
	Timestamp   string `json:"timestamp"`
}) (*Pool, error) {
	// Конвертируем все адреса с проверкой ошибок
	id, err := solana.PublicKeyFromBase58(apiData.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid pool ID: %w", err)
	}

	marketID, err := solana.PublicKeyFromBase58(apiData.MarketID)
	if err != nil {
		return nil, fmt.Errorf("invalid market ID: %w", err)
	}

	lpMint, err := solana.PublicKeyFromBase58(apiData.LPMint)
	if err != nil {
		return nil, fmt.Errorf("invalid LP mint: %w", err)
	}

	creator, err := solana.PublicKeyFromBase58(apiData.Creator)
	if err != nil {
		return nil, fmt.Errorf("invalid creator: %w", err)
	}

	// Парсим временные метки
	openTime, err := strconv.ParseInt(apiData.OpenTimeMs, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid open time: %w", err)
	}

	timestamp, err := time.Parse(time.RFC3339, apiData.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	pool := &Pool{
		ID:          id,
		MarketID:    marketID,
		LPMint:      lpMint,
		Creator:     creator,
		TokenSymbol: apiData.TokenSymbol,
		TokenName:   apiData.TokenName,
		OpenTimeMs:  openTime,
		Timestamp:   timestamp,
		IsFromAPI:   true,
	}

	// Проверяем валидность созданного пула
	if !s.IsPoolValid(pool) {
		return nil, fmt.Errorf("created invalid pool")
	}

	return pool, nil
}
