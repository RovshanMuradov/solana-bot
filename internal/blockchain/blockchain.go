// internal/blockchain/blockchain.go
package blockchain

import (
	"context"
)

type Blockchain interface {
	Name() string
	SendTransaction(ctx context.Context, tx interface{}) (string, error)
	// Другие общие методы
}
