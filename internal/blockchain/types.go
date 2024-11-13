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

// SimulationResult представляет результат симуляции транзакции
// Обновляем структуру SimulationResult
type SimulationResult struct {
	Err           interface{}
	Logs          []string
	UnitsConsumed uint64
}

// Client определяет общий интерфейс для клиентов блокчейна
type Client interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
	GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*rpc.GetAccountInfoResult, error)
	GetSignatureStatuses(ctx context.Context, signatures ...solana.Signature) (*rpc.GetSignatureStatusesResult, error)
	SendTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts TransactionOptions) (solana.Signature, error)
	SimulateTransaction(ctx context.Context, tx *solana.Transaction) (*SimulationResult, error)
	GetTokenAccountBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (*rpc.GetTokenAccountBalanceResult, error)

	// Добавляем новый метод для получения программных аккаунтов
	GetProgramAccounts(ctx context.Context, program solana.PublicKey, opts rpc.GetProgramAccountsOpts) ([]rpc.KeyedAccount, error)

	// Добавляем новый метод для получения баланса
	GetBalance(ctx context.Context, pubkey solana.PublicKey, commitment rpc.CommitmentType) (uint64, error)
	// Добавляем новый метод
	WaitForTransactionConfirmation(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) error
	// Добавляем метод для получения информации о транзакции
	GetTransaction(ctx context.Context, signature solana.Signature) (*rpc.GetTransactionResult, error) //TODO: реализовать этот метод в клиенте
}
