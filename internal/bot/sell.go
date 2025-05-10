// internal/bot/sell.go
package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// SellTokens асинхронно продает токены через DEX
func SellTokens(
	ctx context.Context,
	dexAdapter dex.DEX,
	tokenMint string,
	percent float64,
	slippagePercent float64,
	priorityFee string,
	computeUnits uint32,
	logger *zap.Logger,
) (chan error, error) {
	if dexAdapter == nil {
		return nil, fmt.Errorf("DEX adapter is nil")
	}

	if tokenMint == "" {
		return nil, fmt.Errorf("token mint address is empty")
	}

	// Создаем канал для ошибок, который будет передан вызывающему коду
	errChan := make(chan error, 1)

	// Запускаем продажу в отдельной горутине
	go func() {
		defer close(errChan)

		logger.Info("Starting token sell operation",
			zap.String("token_mint", tokenMint),
			zap.Float64("percent", percent),
			zap.Float64("slippage", slippagePercent),
			zap.String("priority_fee", priorityFee))

		// Создаем отдельный контекст с таймаутом для операции продажи
		sellCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		// Выполняем продажу
		err := dexAdapter.SellPercentTokens(
			sellCtx,
			tokenMint,
			percent,
			slippagePercent,
			priorityFee,
			computeUnits,
		)

		if err != nil {
			logger.Error("Token sell failed", zap.Error(err))
			errChan <- fmt.Errorf("failed to sell tokens: %w", err)
			return
		}

		logger.Info("Token sell completed successfully")
	}()

	return errChan, nil
}

// CreateSellFunc возвращает функцию для продажи токенов
func CreateSellFunc(
	dexAdapter dex.DEX,
	tokenMint string,
	slippagePercent float64,
	priorityFee string,
	computeUnits uint32,
	logger *zap.Logger,
) SellFunc {
	return func(ctx context.Context, percent float64) error {
		errChan, err := SellTokens(
			ctx,
			dexAdapter,
			tokenMint,
			percent,
			slippagePercent,
			priorityFee,
			computeUnits,
			logger,
		)
		if err != nil {
			return err
		}

		// Ожидаем завершения операции продажи или отмены контекста
		select {
		case err := <-errChan:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}