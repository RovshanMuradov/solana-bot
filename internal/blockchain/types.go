// internal/blockchain/types.go
package blockchain

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// TransactionOptions определяет опции для отправки транзакции
type TransactionOptions struct {
	SkipPreflight       bool
	PreflightCommitment rpc.CommitmentType
}

// Client определяет общий интерфейс для клиентов блокчейна
type Client interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
	GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*rpc.GetAccountInfoResult, error)
	GetSignatureStatuses(ctx context.Context, signatures ...solana.Signature) (*rpc.GetSignatureStatusesResult, error)
	SendTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts TransactionOptions) (solana.Signature, error)
}
