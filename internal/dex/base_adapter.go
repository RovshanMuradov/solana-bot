package dex

import (
	"context"
	"sync"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// baseDEXAdapter содержит общую логику для всех адаптеров DEX
type baseDEXAdapter struct {
	client *solbc.Client
	wallet *wallet.Wallet
	logger *zap.Logger
	name   string

	mu        sync.Mutex
	inited    map[string]bool // tokenMint → initialized
	tokenMint string
}

func (b *baseDEXAdapter) init(ctx context.Context, tokenMint string, initFn func() error) error {
	b.mu.Lock()
	if b.inited == nil {
		b.inited = make(map[string]bool)
	}
	if b.inited[tokenMint] {
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()

	// вне lock вызываем initFn (сетевой вызов)
	if err := initFn(); err != nil {
		return err
	}

	// сохраняем факт инициализации под мьютексом
	b.mu.Lock()
	b.inited[tokenMint] = true
	b.tokenMint = tokenMint
	b.mu.Unlock()
	return nil
}

func (b *baseDEXAdapter) GetName() string {
	return b.name
}
