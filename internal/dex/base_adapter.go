// Package dex
// File: internal/dex/factory.go
package dex

import (
	"context"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"sync"

	"go.uber.org/zap"
)

// baseDEXAdapter содержит общую логику для всех адаптеров DEX
type baseDEXAdapter struct {
	client *blockchain.Client
	wallet *task.Wallet
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
