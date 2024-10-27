// internal/blockchain/types.go
package blockchain

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// Client определяет общий интерфейс для клиентов блокчейна
type Client interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
	GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*rpc.GetAccountInfoResult, error)
}
