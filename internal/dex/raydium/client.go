// ======================================
// File: internal/dex/raydium/client.go
// ======================================
package raydium

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// Client provides methods to interact with Raydium for swaps.
type Client struct {
	baseClient     blockchain.Client
	logger         *zap.Logger
	commitment     rpc.CommitmentType
	defaultTimeout time.Duration
}

// NewClient creates a new Raydium client.
// (Currently may not be used in sample code, but can be used in actual project setup.)
func NewClient(baseClient blockchain.Client, logger *zap.Logger) *Client {
	return &Client{
		baseClient:     baseClient,
		logger:         logger.Named("raydium-client"),
		commitment:     rpc.CommitmentConfirmed,
		defaultTimeout: 20 * time.Second,
	}
}

func (c *Client) Swap(ctx context.Context, params *SwapParams) (*SwapResult, error) {
	if params == nil {
		return nil, fmt.Errorf("swap params cannot be nil")
	}
	c.logger.Debug("Preparing swap on Raydium",
		zap.String("source_mint", params.SourceMint.String()),
		zap.String("target_mint", params.TargetMint.String()),
		zap.Uint64("amount_in", params.AmountInLamports))

	instructions, err := BuildSwapInstructions(params, c.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap instructions: %w", err)
	}

	blockhash, err := c.baseClient.GetRecentBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(params.UserPublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	if params.PrivateKey != nil {
		_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(params.UserPublicKey) {
				return params.PrivateKey
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to sign transaction: %w", err)
		}
	}

	sig, err := c.baseClient.SendTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	if params.WaitForConfirmation {
		if err := c.waitForConfirmation(ctx, sig); err != nil {
			return nil, fmt.Errorf("swap transaction not confirmed: %w", err)
		}
	}

	c.logger.Info("Swap transaction sent",
		zap.String("signature", sig.String()),
		zap.Bool("confirmed", params.WaitForConfirmation))
	return &SwapResult{Signature: sig, AmountIn: params.AmountInLamports}, nil
}

func (c *Client) waitForConfirmation(ctx context.Context, sig solana.Signature) error {
	const maxAttempts = 30
	for i := 0; i < maxAttempts; i++ {
		statusResp, err := c.baseClient.GetSignatureStatuses(ctx, sig)
		if err != nil {
			return fmt.Errorf("failed to get signature status: %w", err)
		}
		if statusResp != nil && len(statusResp.Value) > 0 && statusResp.Value[0] != nil {
			if statusResp.Value[0].Err != nil {
				return fmt.Errorf("transaction failed: %v", statusResp.Value[0].Err)
			}
			if statusResp.Value[0].ConfirmationStatus == rpc.ConfirmationStatusFinalized ||
				statusResp.Value[0].ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("confirmation timeout")
}
