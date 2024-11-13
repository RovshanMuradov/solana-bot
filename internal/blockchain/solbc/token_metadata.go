// internal/blockchain/solbc/token_metadata.go
package solbc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

const (
	raydiumAPIEndpoint = "https://api-sm.yvesdev.com/api"
	metadataTTL        = 5 * time.Minute
)

// TokenMetadata хранит информацию о токене
type TokenMetadata struct {
	Decimals  uint8
	Symbol    string
	Name      string
	Price     float64
	Source    string // "chain", "api", "cache"
	UpdatedAt time.Time
}

// TokenMetadataCache управляет кэшированием метаданных токенов
type TokenMetadataCache struct {
	cache      sync.Map
	logger     *zap.Logger
	httpClient *http.Client
}

// RaydiumTokenInfo представляет ответ API Raydium
type RaydiumTokenInfo struct {
	Success bool `json:"success"`
	Token   struct {
		Symbol   string  `json:"symbol"`
		Name     string  `json:"name"`
		Decimals uint8   `json:"decimals"`
		Price    float64 `json:"price"`
	} `json:"token"`
}

func NewTokenMetadataCache(logger *zap.Logger) *TokenMetadataCache {
	return &TokenMetadataCache{
		logger: logger,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetTokenMetadata получает метаданные токена с кэшированием
func (c *TokenMetadataCache) GetTokenMetadata(
	ctx context.Context,
	client blockchain.Client,
	mint solana.PublicKey,
) (*TokenMetadata, error) {
	// 1. Проверяем кэш
	if metadata, ok := c.getFromCache(mint.String()); ok {
		c.logger.Debug("token metadata retrieved from cache",
			zap.String("mint", mint.String()),
			zap.String("symbol", metadata.Symbol))
		return metadata, nil
	}

	// 2. Получаем on-chain данные
	metadata, err := c.getFromChain(ctx, client, mint)
	if err != nil {
		c.logger.Debug("failed to get on-chain metadata",
			zap.String("mint", mint.String()),
			zap.Error(err))
		metadata = &TokenMetadata{} // Создаем пустой объект для дальнейшего обогащения
	}

	// 3. Пробуем обогатить данными из API
	enriched, err := c.enrichFromAPI(ctx, mint, metadata)
	if err != nil {
		c.logger.Debug("failed to enrich metadata from API",
			zap.String("mint", mint.String()),
			zap.Error(err))
	} else {
		metadata = enriched
	}

	// 4. Если всё ещё нет символа/имени, проверяем известные токены
	if metadata.Symbol == "" || metadata.Name == "" {
		c.enrichFromKnownTokens(mint, metadata)
	}

	// 5. Обновляем метаданные и сохраняем в кэш
	metadata.UpdatedAt = time.Now()
	if metadata.Source == "" {
		metadata.Source = "chain"
	}

	c.cache.Store(mint.String(), metadata)

	c.logger.Debug("token metadata retrieved",
		zap.String("mint", mint.String()),
		zap.Uint8("decimals", metadata.Decimals),
		zap.String("symbol", metadata.Symbol),
		zap.String("name", metadata.Name),
		zap.String("source", metadata.Source))

	return metadata, nil
}

// getFromCache получает метаданные из кэша с проверкой TTL
func (c *TokenMetadataCache) getFromCache(mint string) (*TokenMetadata, bool) {
	if value, ok := c.cache.Load(mint); ok {
		metadata := value.(*TokenMetadata)
		if time.Since(metadata.UpdatedAt) < metadataTTL {
			return metadata, true
		}
		// Если данные устарели, удаляем их из кэша
		c.cache.Delete(mint)
	}
	return nil, false
}

// getFromChain получает метаданные из блокчейна
func (c *TokenMetadataCache) getFromChain(
	ctx context.Context,
	client blockchain.Client,
	mint solana.PublicKey,
) (*TokenMetadata, error) {
	acc, err := client.GetAccountInfo(ctx, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get token account: %w", err)
	}

	if acc == nil || acc.Value == nil {
		return nil, fmt.Errorf("token account not found: %s", mint.String())
	}

	data := acc.Value.Data.GetBinary()
	if len(data) < 45 {
		return nil, fmt.Errorf("invalid token account data length: %d", len(data))
	}

	return &TokenMetadata{
		Decimals:  data[44],
		Source:    "chain",
		UpdatedAt: time.Now(),
	}, nil
}

// enrichFromAPI обогащает метаданные данными из API Raydium
func (c *TokenMetadataCache) enrichFromAPI(
	ctx context.Context,
	mint solana.PublicKey,
	metadata *TokenMetadata,
) (*TokenMetadata, error) {
	url := fmt.Sprintf("%s/getToken?token=%s", raydiumAPIEndpoint, mint.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return metadata, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return metadata, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return metadata, fmt.Errorf("API returned status code: %d", resp.StatusCode)
	}

	var tokenInfo RaydiumTokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&tokenInfo); err != nil {
		return metadata, fmt.Errorf("failed to decode API response: %w", err)
	}

	if !tokenInfo.Success {
		return metadata, fmt.Errorf("API returned unsuccessful response")
	}

	// Обновляем только если получили новые данные
	if tokenInfo.Token.Symbol != "" {
		metadata.Symbol = tokenInfo.Token.Symbol
	}
	if tokenInfo.Token.Name != "" {
		metadata.Name = tokenInfo.Token.Name
	}
	if tokenInfo.Token.Decimals > 0 {
		metadata.Decimals = tokenInfo.Token.Decimals
	}
	metadata.Price = tokenInfo.Token.Price
	metadata.Source = "api"

	return metadata, nil
}

// enrichFromKnownTokens обогащает метаданные для известных токенов
func (c *TokenMetadataCache) enrichFromKnownTokens(mint solana.PublicKey, metadata *TokenMetadata) {
	switch mint.String() {
	case "So11111111111111111111111111111111111111112": // wSOL
		metadata.Symbol = "SOL"
		metadata.Name = "Wrapped SOL"
	case "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v": // USDC
		metadata.Symbol = "USDC"
		metadata.Name = "USD Coin"
	case "DezXAZ8z7PnrnRJjz3wXBoRgixCa6xjnB7YaB1pPB263": // Bonk
		metadata.Symbol = "BONK"
		metadata.Name = "Bonk"
	}
}
