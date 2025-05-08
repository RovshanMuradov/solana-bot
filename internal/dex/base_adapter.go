// Package dex =============================
// File: internal/dex/base_adapter.go
// =============================
package dex

import (
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
	"sync"
)

// baseDEXAdapter содержит общую логику для всех адаптеров DEX
type baseDEXAdapter struct {
	client    *solbc.Client
	wallet    *wallet.Wallet
	logger    *zap.Logger
	name      string
	tokenMint string
	initMu    sync.Mutex
	initDone  bool
}

// GetName возвращает название биржи
func (b *baseDEXAdapter) GetName() string {
	return b.name
}
