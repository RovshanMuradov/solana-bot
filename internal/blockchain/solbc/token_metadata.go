// internal/blockchain/solana/token_metadata.go
package solbc

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// TokenMetadata хранит информацию о токене
type TokenMetadata struct {
	Decimals uint8
	Symbol   string
	Name     string
}

func NewTokenMetadataCache(logger *zap.Logger) *TokenMetadataCache {
	return &TokenMetadataCache{
		logger: logger,
	}
}

// GetTokenMetadata получает метаданные токена с кэшированием
func (c *TokenMetadataCache) GetTokenMetadata(
	ctx context.Context,
	client blockchain.Client,
	mint solana.PublicKey,
) (*TokenMetadata, error) {
	// Проверяем кэш
	if metadata, ok := c.cache.Load(mint.String()); ok {
		return metadata.(*TokenMetadata), nil
	}

	// Получаем аккаунт токена
	acc, err := client.GetAccountInfo(ctx, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get token account: %w", err)
	}

	if acc == nil || acc.Value == nil {
		return nil, fmt.Errorf("token account not found: %s", mint.String())
	}

	// Парсим данные аккаунта
	data := acc.Value.Data.GetBinary()
	if len(data) < 45 { // Минимальная длина для получения decimals
		return nil, fmt.Errorf("invalid token account data length: %d", len(data))
	}

	metadata := &TokenMetadata{
		Decimals: data[44], // Decimals находится на 44 байте
	}

	// Добавляем информацию для известных токенов
	switch mint.String() {
	case "So11111111111111111111111111111111111111112": // wSOL
		metadata.Symbol = "SOL"
		metadata.Name = "Wrapped SOL"
	case "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v": // USDC
		metadata.Symbol = "USDC"
		metadata.Name = "USD Coin"
	}

	// Сохраняем в кэш
	c.cache.Store(mint.String(), metadata)

	c.logger.Debug("Token metadata retrieved",
		zap.String("mint", mint.String()),
		zap.Uint8("decimals", metadata.Decimals),
		zap.String("symbol", metadata.Symbol),
		zap.String("name", metadata.Name))

	return metadata, nil
}

// GetMultipleTokenMetadata получает метаданные нескольких токенов параллельно
func (c *TokenMetadataCache) GetMultipleTokenMetadata(
	ctx context.Context,
	client blockchain.Client,
	mints []solana.PublicKey,
) (map[string]*TokenMetadata, error) {
	result := make(map[string]*TokenMetadata)
	var wg sync.WaitGroup
	errCh := make(chan error, len(mints))
	resultCh := make(chan struct {
		mint     string
		metadata *TokenMetadata
	}, len(mints))

	for _, mint := range mints {
		wg.Add(1)
		go func(m solana.PublicKey) {
			defer wg.Done()

			metadata, err := c.GetTokenMetadata(ctx, client, m)
			if err != nil {
				errCh <- fmt.Errorf("failed to get metadata for %s: %w", m.String(), err)
				return
			}

			resultCh <- struct {
				mint     string
				metadata *TokenMetadata
			}{m.String(), metadata}
		}(mint)
	}

	// Запускаем горутину для закрытия каналов после завершения всех запросов
	go func() {
		wg.Wait()
		close(errCh)
		close(resultCh)
	}()

	// Собираем результаты и ошибки
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	for r := range resultCh {
		result[r.mint] = r.metadata
	}

	if len(errs) > 0 {
		return result, fmt.Errorf("some tokens failed: %v", errs)
	}

	return result, nil
}

// ClearCache очищает кэш метаданных
func (c *TokenMetadataCache) ClearCache() {
	c.cache = sync.Map{}
}
