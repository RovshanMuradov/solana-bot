// =============================
// File: internal/dex/dex.go
// =============================
package dex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/utils/metrics"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// DEX — единый интерфейс для работы с различными DEX.
type DEX interface {
	// GetName возвращает название биржи.
	GetName() string
	// Execute выполняет операцию, описанную в задаче.
	Execute(ctx context.Context, task *Task) error
}

// pumpfunDEXAdapter – адаптер для Pump.fun, реализующий интерфейс DEX.
type pumpfunDEXAdapter struct {
	inner   *pumpfun.DEX
	logger  *zap.Logger
	metrics *metrics.Collector
}

func (d *pumpfunDEXAdapter) GetName() string {
	return "Pump.fun"
}

func (d *pumpfunDEXAdapter) Execute(ctx context.Context, task *Task) error {
	start := time.Now()
	var txType string

	switch task.Operation {
	case OperationSnipe:
		txType = "snipe"
		d.logger.Info("Executing snipe on Pump.fun")
		err := d.inner.ExecuteSnipe(ctx, task.Amount, task.MinSolOutput)
		d.metrics.RecordTransaction(ctx, txType, d.GetName(), time.Since(start), err == nil)
		return err
	case OperationSell:
		txType = "sell"
		d.logger.Info("Executing sell on Pump.fun")
		err := d.inner.ExecuteSell(ctx, task.Amount, task.MinSolOutput)
		d.metrics.RecordTransaction(ctx, txType, d.GetName(), time.Since(start), err == nil)
		return err
	default:
		err := fmt.Errorf("operation %s is not supported on Pump.fun", task.Operation)
		d.metrics.RecordTransaction(ctx, "unsupported", d.GetName(), time.Since(start), false)
		return err
	}
}

// raydiumDEXAdapter – адаптер для Raydium, реализующий интерфейс DEX.
type raydiumDEXAdapter struct {
	client  *raydium.Client
	logger  *zap.Logger
	metrics *metrics.Collector
}

func (d *raydiumDEXAdapter) GetName() string {
	return "Raydium"
}

func (d *raydiumDEXAdapter) Execute(ctx context.Context, task *Task) error {
	start := time.Now()

	switch task.Operation {
	case OperationSwap, OperationSnipe:
		d.logger.Info("Executing swap/snipe on Raydium")
		// Здесь для демонстрации используется один и тот же набор параметров.
		snipeParams := &raydium.SnipeParams{
			TokenMint:           solana.PublicKey{}, // Здесь следует указать реальный mint
			SourceMint:          solana.MustPublicKeyFromBase58("SOURCE_MINT"),
			AmmAuthority:        solana.MustPublicKeyFromBase58("AMM_AUTHORITY"),
			BaseVault:           solana.MustPublicKeyFromBase58("BASE_VAULT"),
			QuoteVault:          solana.MustPublicKeyFromBase58("QUOTE_VAULT"),
			UserPublicKey:       solana.PublicKey{}, // Здесь — публичный ключ пользователя
			PrivateKey:          nil,                // При необходимости — приватный ключ
			UserSourceATA:       solana.MustPublicKeyFromBase58("USER_SOURCE_ATA"),
			UserDestATA:         solana.MustPublicKeyFromBase58("USER_DEST_ATA"),
			AmountInLamports:    task.Amount,
			MinOutLamports:      task.MinSolOutput,
			PriorityFeeLamports: 0,
		}

		err := func() error {
			_, err := d.client.Snipe(ctx, snipeParams)
			return err
		}()
		// В данном случае операция именуется "swap"
		d.metrics.RecordTransaction(ctx, "swap", d.GetName(), time.Since(start), err == nil)
		return err
	default:
		err := fmt.Errorf("operation %s is not supported on Raydium", task.Operation)
		d.metrics.RecordTransaction(ctx, "unsupported", d.GetName(), time.Since(start), false)
		return err
	}
}

// GetDEXByName создаёт адаптер для DEX по имени биржи.
// Теперь функция принимает дополнительный параметр metricsCollector.
func GetDEXByName(name string, client interface{}, w *wallet.Wallet, logger *zap.Logger, mc *metrics.Collector) (DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet cannot be nil")
	}
	if mc == nil {
		return nil, fmt.Errorf("metrics collector cannot be nil")
	}

	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "pump.fun":
		solClient, ok := client.(*solbc.Client)
		if !ok {
			return nil, fmt.Errorf("invalid client type for Pump.fun; *solbc.Client required")
		}
		// Получаем дефолтную конфигурацию для Pump.fun
		config := pumpfun.GetDefaultConfig(logger)
		// Передаём экземпляр кошелька в конструктор NewDEX
		pfDex, err := pumpfun.NewDEX(solClient, w, logger, config, config.MonitorInterval)
		if err != nil {
			return nil, fmt.Errorf("could not create DEX for Pump.fun: %w", err)
		}
		return &pumpfunDEXAdapter{
			inner:   pfDex,
			logger:  logger,
			metrics: mc,
		}, nil

	case "raydium":
		raydClient, ok := client.(*raydium.Client)
		if !ok {
			return nil, fmt.Errorf("invalid client type for Raydium; *raydium.Client required")
		}
		return &raydiumDEXAdapter{
			client:  raydClient,
			logger:  logger,
			metrics: mc,
		}, nil

	default:
		return nil, fmt.Errorf("exchange %s is not supported", name)
	}
}
