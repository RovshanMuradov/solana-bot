// internal/dex/raydium/client_swap.go
package raydium

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// Swap выполняет одну транзакцию свапа с расширенной валидацией и мониторингом
func (c *Client) Swap(ctx context.Context, params *SwapParams) (solana.Signature, error) {
	if params == nil {
		return solana.Signature{}, fmt.Errorf("swap params cannot be nil")
	}

	// Подготовка и валидация параметров свапа
	if err := c.prepareSwap(ctx, params); err != nil {
		return solana.Signature{}, fmt.Errorf("swap preparation failed: %w", err)
	}

	// Получаем последний блокхэш
	recentBlockHash, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Подготавливаем инструкции через единый метод
	instructions, err := c.PrepareSwapInstructions(params)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to prepare swap instructions: %w", err)
	}

	// Создаем транзакцию со всеми подготовленными инструкциями
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockHash,
		solana.TransactionPayer(params.UserWallet),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию если предоставлен приватный ключ
	if params.PrivateKey != nil {
		if _, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(params.UserWallet) {
				return params.PrivateKey
			}
			return nil
		}); err != nil {
			return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
		}
	}

	// Отправляем и получаем результат
	sig, err := c.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	// Ждем подтверждения если требуется
	if params.WaitConfirmation {
		if err := c.WaitForConfirmation(ctx, sig); err != nil {
			return sig, fmt.Errorf("transaction confirmation failed: %w", err)
		}
	}

	// Обновляем состояние пула в кэше
	if err := c.poolCache.UpdatePoolState(params.Pool); err != nil {
		c.logger.Warn("failed to update pool state in cache",
			zap.Error(err),
			zap.String("pool_id", params.Pool.ID.String()))
	}

	return sig, nil
}

// WaitForConfirmation ждет подтверждения транзакции
func (c *Client) WaitForConfirmation(ctx context.Context, sig solana.Signature) error {
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			status, err := c.client.GetSignatureStatuses(ctx, sig)
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}
			if status != nil && len(status.Value) > 0 && status.Value[0] != nil {
				if status.Value[0].Err != nil {
					return fmt.Errorf("transaction failed: %v", status.Value[0].Err)
				}
				if status.Value[0].ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
					status.Value[0].ConfirmationStatus == rpc.ConfirmationStatusFinalized {
					return nil
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	return fmt.Errorf("confirmation timeout")
}
