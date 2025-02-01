// internal/dex/dex.go
package dex

import (
	"context"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
)

// DEX — общий интерфейс для работы с биржами (в данном случае — только Pump.fun).
type DEX interface {
	GetName() string
	ExecuteSnipe(ctx context.Context, tokenContract solana.PublicKey) error
	ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error
	CheckForGraduation(ctx context.Context) (bool, error)
}

// pumpfunDEX — адаптер для Pump.fun, реализующий интерфейс DEX.
type pumpfunDEX struct {
	inner *pumpfun.DEX
}

func (d *pumpfunDEX) GetName() string {
	return "Pump.fun"
}

func (d *pumpfunDEX) ExecuteSnipe(ctx context.Context, tokenContract solana.PublicKey) error {
	return d.inner.ExecuteSnipe(ctx, tokenContract)
}

func (d *pumpfunDEX) ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error {
	return d.inner.ExecuteSell(ctx, amount, minSolOutput)
}

func (d *pumpfunDEX) CheckForGraduation(ctx context.Context) (bool, error) {
	return d.inner.CheckForGraduation(ctx)
}

// GetDEXByName возвращает экземпляр DEX по имени.
// В данной реализации поддерживается только "pump.fun".
func GetDEXByName(name string, client interface{}, logger *zap.Logger) (DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	name = strings.ToLower(strings.TrimSpace(name))
	if name != "pump.fun" {
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}

	// Приводим клиент к типу *solbc.Client (для работы с Solana).
	solClient, ok := client.(*solbc.Client)
	if !ok {
		return nil, fmt.Errorf("invalid client type for pumpfun DEX")
	}

	// Получаем конфигурацию для Pump.fun из отдельного модуля.
	config := pumpfun.GetDefaultConfig(logger)

	// Создаём экземпляр DEX для Pump.fun.
	pfDex, err := pumpfun.NewDEX(solClient, logger, config, config.MonitorInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to create pumpfun DEX: %w", err)
	}

	return &pumpfunDEX{inner: pfDex}, nil
}
