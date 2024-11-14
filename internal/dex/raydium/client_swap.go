// internal/dex/raydium/client_swap.go
package raydium

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// Swap выполняет одну транзакцию свапа
func (c *Client) Swap(ctx context.Context, params *SwapParams) (solana.Signature, error) {
	if params == nil {
		return solana.Signature{}, fmt.Errorf("swap params cannot be nil")
	}

	// Используем новый объединенный метод валидации и подготовки
	if err := c.validateAndPrepareSwap(ctx, params); err != nil {
		return solana.Signature{}, fmt.Errorf("swap validation failed: %w", err)
	}

	// Получаем последний блокхэш
	recentBlockHash, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Подготавливаем инструкции
	instructions, err := c.PrepareSwapInstructions(params)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to prepare instructions: %w", err)
	}

	// Создаем и подписываем транзакцию
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockHash,
		solana.TransactionPayer(params.UserWallet),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию
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

	// Отправляем транзакцию
	sig, err := c.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	// Ждем подтверждения если требуется
	if params.WaitConfirmation {
		if err := c.WaitForConfirmation(ctx, sig); err != nil {
			return sig, err
		}
	}

	return sig, nil
}

// WaitForConfirmation ждет подтверждения транзакции с таймаутом
func (c *Client) WaitForConfirmation(ctx context.Context, sig solana.Signature) error {
	const (
		maxAttempts   = 30
		checkInterval = 500 * time.Millisecond
	)

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(checkInterval):
			status, err := c.client.GetSignatureStatuses(ctx, sig)
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			if status == nil || len(status.Value) == 0 || status.Value[0] == nil {
				continue
			}

			if status.Value[0].Err != nil {
				return fmt.Errorf("transaction failed: %v", status.Value[0].Err)
			}

			if status.Value[0].ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
				status.Value[0].ConfirmationStatus == rpc.ConfirmationStatusFinalized {
				return nil
			}
		}
	}

	return fmt.Errorf("confirmation timeout after %d attempts", maxAttempts)
}
