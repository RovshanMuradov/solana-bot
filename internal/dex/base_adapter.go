// Package dex =============================
// File: internal/dex/base_adapter.go
// =============================
package dex

import (
	"context"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
	"sync"
)

// baseDEXAdapter содержит общую логику для всех адаптеров DEX
type baseDEXAdapter struct {
	client *solbc.Client
	wallet *wallet.Wallet
	logger *zap.Logger
	name   string

	mu        sync.Mutex
	initDone  bool
	tokenMint string
}

// initIfNeeded выполняет «ленивую» инициализацию для конкретного токена.
// initFn — это ваша «фабрика», которая создаёт inner DEX.
func (b *baseDEXAdapter) initIfNeeded(ctx context.Context, tokenMint string, initFn func() error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initDone && b.tokenMint == tokenMint {
		return nil
	}
	if err := initFn(); err != nil {
		return err
	}
	b.tokenMint = tokenMint
	b.initDone = true
	return nil
}

// GetName возвращает название биржи
func (b *baseDEXAdapter) GetName() string {
	return b.name
}
