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

// Execute выполняет операцию, описанную в задаче.
func (d *pumpfunDEXAdapter) Execute(ctx context.Context, task *Task) error {
	start := time.Now()
	var txType string

	// Проверяем, указан ли адрес токена
	if task.TokenMint == "" {
		err := fmt.Errorf("token mint address is required for Pump.fun")
		d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
		return err
	}

	// Use token mint address as is, without any modification
	tokenMint := task.TokenMint

	// Если DEX еще не инициализирован, создаем его с токеном из задачи
	if d.inner == nil {
		solClient, ok := d.metrics.GetSolanaClient()
		if !ok {
			err := fmt.Errorf("solana client not available")
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return err
		}

		// Получаем конфигурацию и настраиваем для указанного токена
		config := pumpfun.GetDefaultConfig()
		if err := config.SetupForToken(tokenMint, d.logger); err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("failed to setup token config: %w", err)
		}

		// Получаем кошелек пользователя
		wallet, err := d.metrics.GetUserWallet()
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("failed to get user wallet: %w", err)
		}

		// Создаем DEX с конфигурацией для токена
		var dexErr error
		d.inner, dexErr = pumpfun.NewDEX(solClient, wallet, d.logger, config, config.MonitorInterval)
		if dexErr != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("failed to initialize Pump.fun DEX: %w", dexErr)
		}
	}

	// Выполняем операцию
	switch task.Operation {
	case OperationSnipe:
		txType = "snipe"
		d.logger.Info("Executing snipe on Pump.fun",
			zap.String("token_mint", tokenMint),
			zap.Uint64("amount", task.Amount))
		err := d.inner.ExecuteSnipe(ctx, task.Amount, task.MinSolOutput)
		d.metrics.RecordTransaction(txType, d.GetName(), time.Since(start), err == nil)
		return err
	case OperationSell:
		txType = "sell"
		d.logger.Info("Executing sell on Pump.fun",
			zap.String("token_mint", tokenMint),
			zap.Uint64("amount", task.Amount))
		err := d.inner.ExecuteSell(ctx, task.Amount, task.MinSolOutput)
		d.metrics.RecordTransaction(txType, d.GetName(), time.Since(start), err == nil)
		return err
	default:
		err := fmt.Errorf("operation %s is not supported on Pump.fun", task.Operation)
		d.metrics.RecordTransaction("unsupported", d.GetName(), time.Since(start), false)
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

	// Use token mint address as is, without any modification
	tokenMint := task.TokenMint

	switch task.Operation {
	case OperationSwap, OperationSnipe:
		d.logger.Info("Executing swap/snipe on Raydium")

		// Безопасное преобразование адреса токена
		tokenMintKey, err := solana.PublicKeyFromBase58(tokenMint)
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("invalid token mint address: %w", err)
		}

		// Используем безопасные версии для константных значений
		sourceMint, err := solana.PublicKeyFromBase58("SOURCE_MINT")
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("invalid source mint: %w", err)
		}

		ammAuthority, err := solana.PublicKeyFromBase58("AMM_AUTHORITY")
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("invalid amm authority: %w", err)
		}

		baseVault, err := solana.PublicKeyFromBase58("BASE_VAULT")
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("invalid base vault: %w", err)
		}

		quoteVault, err := solana.PublicKeyFromBase58("QUOTE_VAULT")
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("invalid quote vault: %w", err)
		}

		userSourceATA, err := solana.PublicKeyFromBase58("USER_SOURCE_ATA")
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("invalid user source ATA: %w", err)
		}

		userDestATA, err := solana.PublicKeyFromBase58("USER_DEST_ATA")
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("invalid user destination ATA: %w", err)
		}

		// Здесь для демонстрации используется один и тот же набор параметров
		snipeParams := &raydium.SnipeParams{
			TokenMint:           tokenMintKey, // Используем безопасно преобразованный ключ
			SourceMint:          sourceMint,
			AmmAuthority:        ammAuthority,
			BaseVault:           baseVault,
			QuoteVault:          quoteVault,
			UserPublicKey:       solana.PublicKey{}, // Здесь — публичный ключ пользователя
			PrivateKey:          nil,                // При необходимости — приватный ключ
			UserSourceATA:       userSourceATA,
			UserDestATA:         userDestATA,
			AmountInLamports:    task.Amount,
			MinOutLamports:      task.MinSolOutput,
			PriorityFeeLamports: 0,
		}

		err = func() error {
			_, err := d.client.Snipe(ctx, snipeParams)
			return err
		}()
		// В данном случае операция именуется "swap"
		d.metrics.RecordTransaction("swap", d.GetName(), time.Since(start), err == nil)
		return err
	default:
		err := fmt.Errorf("operation %s is not supported on Raydium", task.Operation)
		d.metrics.RecordTransaction("unsupported", d.GetName(), time.Since(start), false)
		return err
	}
}

// GetDEXByName создаёт адаптер для DEX по имени биржи.
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
		// Check client type compatibility only
		_, ok := client.(*solbc.Client)
		if !ok {
			return nil, fmt.Errorf("invalid client type for Pump.fun; *solbc.Client required")
		}

		// Return adapter without initializing DEX
		// DEX will be initialized in Execute() method when task.TokenMint is available
		return &pumpfunDEXAdapter{
			inner:   nil,
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
