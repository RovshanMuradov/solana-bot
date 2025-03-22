// =============================
// File: internal/dex/dex.go
// =============================
package dex

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
	"github.com/rovshanmuradov/solana-bot/internal/utils/metrics"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// DEX — единый интерфейс для работы с различными DEX.
type DEX interface {
	// GetName возвращает название биржи.
	GetName() string
	// Execute выполняет операцию, описанную в задаче.
	Execute(ctx context.Context, task *Task) error
	// GetTokenPrice возвращает текущую цену токена (новый метод)
	GetTokenPrice(ctx context.Context, tokenMint string) (float64, error)
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
			zap.Float64("tokens_to_sell", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		err := d.inner.ExecuteSnipe(ctx, task.AmountSol, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		d.metrics.RecordTransaction(txType, d.GetName(), time.Since(start), err == nil)
		return err

	case OperationSell:
		txType = "sell"

		// Допустим, у вашего токена 6 десятичных знаков.
		decimals := 6.0

		// Преобразуем значение из tasks.csv (человеко-читаемый формат)
		// в базовые единицы: например, "1564784.000000" -> 1564784 * 10^6.
		tokenAmount := uint64(task.AmountSol * math.Pow(10, decimals))

		d.logger.Info("Executing sell on Pump.fun",
			zap.String("token_mint", tokenMint),
			zap.Float64("tokens_to_sell", task.AmountSol),
			zap.Uint64("token_amount", tokenAmount),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		err := d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		d.metrics.RecordTransaction(txType, d.GetName(), time.Since(start), err == nil)
		return err

	default:
		err := fmt.Errorf("operation %s is not supported on Pump.fun", task.Operation)
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

	case "pump.swap":
		_, ok := client.(*solbc.Client)
		if !ok {
			return nil, fmt.Errorf("invalid client type for Pump.Swap; *solbc.Client required")
		}

		return &pumpswapDEXAdapter{
			inner:   nil, // Will be initialized in Execute with token mint
			logger:  logger,
			metrics: mc,
		}, nil

	default:
		return nil, fmt.Errorf("exchange %s is not supported", name)
	}
}

// GetTokenPrice returns the current price of the token
func (d *pumpfunDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Если DEX еще не инициализирован, создаем его с токеном из запроса
	if d.inner == nil {
		solClient, ok := d.metrics.GetSolanaClient()
		if !ok {
			return 0, fmt.Errorf("solana client not available")
		}

		// Получаем конфигурацию и настраиваем для указанного токена
		config := pumpfun.GetDefaultConfig()
		if err := config.SetupForToken(tokenMint, d.logger); err != nil {
			return 0, fmt.Errorf("failed to setup token config: %w", err)
		}

		// Получаем кошелек пользователя
		wallet, err := d.metrics.GetUserWallet()
		if err != nil {
			return 0, fmt.Errorf("failed to get user wallet: %w", err)
		}

		// Создаем DEX с конфигурацией для токена
		var dexErr error
		d.inner, dexErr = pumpfun.NewDEX(solClient, wallet, d.logger, config, "5s") // Default monitor interval
		if dexErr != nil {
			return 0, fmt.Errorf("failed to initialize Pump.fun DEX: %w", dexErr)
		}
	}

	// Вызываем метод внутреннего DEX
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// pumpswapDEXAdapter is the adapter for PumpSwap
type pumpswapDEXAdapter struct {
	inner   *pumpswap.DEX
	logger  *zap.Logger
	metrics *metrics.Collector
}

// GetName returns the exchange name
func (d *pumpswapDEXAdapter) GetName() string {
	return "Pump.Swap"
}

// Execute implements the execution of tasks for PumpSwap DEX
func (d *pumpswapDEXAdapter) Execute(ctx context.Context, task *Task) error {
	start := time.Now()
	var txType string

	// Check if token mint is provided
	if task.TokenMint == "" {
		err := fmt.Errorf("token mint address is required for Pump.Swap")
		d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
		return err
	}

	// Use token mint address as is
	tokenMint := task.TokenMint

	// Initialize DEX if not already done
	if d.inner == nil {
		solClient, ok := d.metrics.GetSolanaClient()
		if !ok {
			err := fmt.Errorf("solana client not available")
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return err
		}

		// Get default config and set up for token
		config := pumpswap.GetDefaultConfig()
		if err := config.SetupForToken(tokenMint, d.logger); err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("failed to setup token config: %w", err)
		}

		// Get user wallet
		wallet, err := d.metrics.GetUserWallet()
		if err != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("failed to get user wallet: %w", err)
		}

		// Create new DEX instance
		var dexErr error
		d.inner, dexErr = pumpswap.NewDEX(solClient, wallet, d.logger, config, config.MonitorInterval)
		if dexErr != nil {
			d.metrics.RecordTransaction("error", d.GetName(), time.Since(start), false)
			return fmt.Errorf("failed to initialize Pump.Swap DEX: %w", dexErr)
		}
	}

	// Execute operation based on task type
	switch task.Operation {
	case OperationSnipe:
		txType = "snipe"

		d.logger.Info("Executing snipe on Pump.Swap",
			zap.String("token_mint", tokenMint),
			zap.Float64("amount_sol", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		err := d.inner.ExecuteSnipe(ctx, task.AmountSol, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		d.metrics.RecordTransaction(txType, d.GetName(), time.Since(start), err == nil)
		return err
		
	case OperationSwap:
		txType = "swap"

		d.logger.Info("Executing swap on Pump.Swap",
			zap.String("token_mint", tokenMint),
			zap.Float64("amount_sol", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		// Convert SOL amount to lamports for swap
		amountLamports := uint64(task.AmountSol * 1e9)
		
		// Execute swap (buy) operation - assuming user has WSOL and wants to buy the token
		err := d.inner.ExecuteSwap(ctx, true, amountLamports, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		d.metrics.RecordTransaction(txType, d.GetName(), time.Since(start), err == nil)
		return err

	case OperationSell:
		txType = "sell"

		// Get token precision to adjust amount
		tokenMintPubkey, err := solana.PublicKeyFromBase58(tokenMint)
		if err != nil {
			return fmt.Errorf("invalid token mint address: %w", err)
		}

		// Determine token precision
		precision, err := d.inner.DetermineTokenPrecision(ctx, tokenMintPubkey)
		if err != nil {
			precision = 6 // Default precision if cannot determine
			d.logger.Warn("Could not determine token precision, using default",
				zap.Uint8("default_precision", precision))
		}

		// Convert amount to token units
		tokenAmount := uint64(task.AmountSol * math.Pow(10, float64(precision)))

		d.logger.Info("Executing sell on Pump.Swap",
			zap.String("token_mint", tokenMint),
			zap.Float64("tokens_to_sell", task.AmountSol),
			zap.Uint64("token_amount", tokenAmount),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		err = d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		d.metrics.RecordTransaction(txType, d.GetName(), time.Since(start), err == nil)
		return err

	default:
		err := fmt.Errorf("operation %s is not supported on Pump.Swap", task.Operation)
		d.metrics.RecordTransaction("unsupported", d.GetName(), time.Since(start), false)
		return err
	}
}

// GetTokenPrice returns the current price of the token
func (d *pumpswapDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Initialize DEX if not already done
	if d.inner == nil {
		solClient, ok := d.metrics.GetSolanaClient()
		if !ok {
			return 0, fmt.Errorf("solana client not available")
		}

		// Get default config and set up for token
		config := pumpswap.GetDefaultConfig()
		if err := config.SetupForToken(tokenMint, d.logger); err != nil {
			return 0, fmt.Errorf("failed to setup token config: %w", err)
		}

		// Get user wallet
		wallet, err := d.metrics.GetUserWallet()
		if err != nil {
			return 0, fmt.Errorf("failed to get user wallet: %w", err)
		}

		// Create new DEX instance
		var dexErr error
		d.inner, dexErr = pumpswap.NewDEX(solClient, wallet, d.logger, config, "5s") // Default monitor interval
		if dexErr != nil {
			return 0, fmt.Errorf("failed to initialize Pump.Swap DEX: %w", dexErr)
		}
	}

	// Call inner DEX method
	return d.inner.GetTokenPrice(ctx, tokenMint)
}