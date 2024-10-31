// internal/blockchain/types.go
package blockchain

import (
	"context"
	"encoding/base64"

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
	ReturnData    *Base64Data
}

// Base64Data представляет данные в формате Base64
type Base64Data struct {
	Data string
}

// EncodeBase64 кодирует данные в Base64
func (b *Base64Data) EncodeBase64(data []byte) {
	b.Data = base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 декодирует данные из Base64
func (b *Base64Data) DecodeBase64() ([]byte, error) {
	return base64.StdEncoding.DecodeString(b.Data)
}

// Client определяет общий интерфейс для клиентов блокчейна
type Client interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
	GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*rpc.GetAccountInfoResult, error)
	GetSignatureStatuses(ctx context.Context, signatures ...solana.Signature) (*rpc.GetSignatureStatusesResult, error)
	SendTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts TransactionOptions) (solana.Signature, error)
	SimulateTransaction(ctx context.Context, tx *solana.Transaction) (*SimulationResult, error)
	GetRpcClient() *rpc.Client
}
