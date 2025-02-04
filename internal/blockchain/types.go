// internal/blockchain/types.go
package blockchain

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// TransactionOptions определяет опции для отправки транзакций.
type TransactionOptions struct {
	SkipPreflight       bool
	PreflightCommitment rpc.CommitmentType
}

// SimulationResult представляет результат симуляции транзакции.
type SimulationResult struct {
	Err           interface{}
	Logs          []string
	UnitsConsumed uint64
}

// Client определяет общий интерфейс для взаимодействия с блокчейном.
type Client interface {
	// Получить последний blockhash.
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	// Отправить транзакцию.
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
	// Получить информацию об аккаунте.
	GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*rpc.GetAccountInfoResult, error)
	// Получить статусы подписей транзакций.
	GetSignatureStatuses(ctx context.Context, signatures ...solana.Signature) (*rpc.GetSignatureStatusesResult, error)
	// Отправить транзакцию с опциями.
	SendTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts TransactionOptions) (solana.Signature, error)
	// Симулировать транзакцию.
	SimulateTransaction(ctx context.Context, tx *solana.Transaction) (*SimulationResult, error)
	// Получить баланс аккаунта.
	GetBalance(ctx context.Context, pubkey solana.PublicKey, commitment rpc.CommitmentType) (uint64, error)
	// Ожидание подтверждения транзакции.
	WaitForTransactionConfirmation(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) error
}
