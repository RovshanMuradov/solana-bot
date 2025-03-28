// =============================
// File: internal/dex/adapters.go
// =============================
package dex

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
	"math"
	"strings"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// pumpfunDEXAdapter – адаптер для Pump.fun, реализующий интерфейс DEX.
type pumpfunDEXAdapter struct {
	inner  *pumpfun.DEX
	logger *zap.Logger
	client *solbc.Client
	wallet *wallet.Wallet
}

func (d *pumpfunDEXAdapter) GetName() string {
	return "Pump.fun"
}

// Execute выполняет операцию, описанную в задаче.
func (d *pumpfunDEXAdapter) Execute(ctx context.Context, task *Task) error {
	// Проверяем, указан ли адрес токена
	if task.TokenMint == "" {
		return fmt.Errorf("token mint address is required for Pump.fun")
	}

	tokenMint := task.TokenMint

	// Если DEX еще не инициализирован, создаем его с токеном из задачи
	if d.inner == nil {
		// Получаем конфигурацию и настраиваем для указанного токена
		config := pumpfun.GetDefaultConfig()
		if err := config.SetupForToken(tokenMint, d.logger); err != nil {
			return fmt.Errorf("failed to setup token config: %w", err)
		}

		// Создаем DEX с конфигурацией для токена
		var dexErr error
		d.inner, dexErr = pumpfun.NewDEX(d.client, d.wallet, d.logger, config, config.MonitorInterval)
		if dexErr != nil {
			return fmt.Errorf("failed to initialize Pump.fun DEX: %w", dexErr)
		}
	}

	// Выполняем операцию
	switch task.Operation {
	case OperationSnipe:
		d.logger.Info("Executing snipe on Pump.fun",
			zap.String("token_mint", tokenMint),
			zap.Float64("sol_amount", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		return d.inner.ExecuteSnipe(ctx, task.AmountSol, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	case OperationSell:
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

		return d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	default:
		return fmt.Errorf("operation %s is not supported on Pump.fun", task.Operation)
	}
}

// GetDEXByName создаёт адаптер для DEX по имени биржи.
func GetDEXByName(name string, client *solbc.Client, w *wallet.Wallet, logger *zap.Logger) (DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet cannot be nil")
	}

	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "pump.fun":
		return &pumpfunDEXAdapter{
			inner:  nil,
			logger: logger,
			client: client,
			wallet: w,
		}, nil

	case "pump.swap":
		return &pumpswapDEXAdapter{
			inner:  nil,
			logger: logger,
			client: client,
			wallet: w,
		}, nil

	default:
		return nil, fmt.Errorf("exchange %s is not supported", name)
	}
}

// GetTokenPrice returns the current price of the token
func (d *pumpfunDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Если DEX еще не инициализирован, создаем его с токеном из запроса
	if d.inner == nil {
		// Получаем конфигурацию и настраиваем для указанного токена
		config := pumpfun.GetDefaultConfig()
		if err := config.SetupForToken(tokenMint, d.logger); err != nil {
			return 0, fmt.Errorf("failed to setup token config: %w", err)
		}

		// Создаем DEX с конфигурацией для токена
		var dexErr error
		d.inner, dexErr = pumpfun.NewDEX(d.client, d.wallet, d.logger, config, "5s") // Default monitor interval
		if dexErr != nil {
			return 0, fmt.Errorf("failed to initialize Pump.fun DEX: %w", dexErr)
		}
	}

	// Вызываем метод внутреннего DEX
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// pumpswapDEXAdapter is the adapter for PumpSwap
type pumpswapDEXAdapter struct {
	inner  *pumpswap.DEX
	logger *zap.Logger
	client *solbc.Client
	wallet *wallet.Wallet
}

// GetName returns the exchange name
func (d *pumpswapDEXAdapter) GetName() string {
	return "Pump.Swap"
}

// Execute implements the execution of tasks for PumpSwap DEX
func (d *pumpswapDEXAdapter) Execute(ctx context.Context, task *Task) error {
	// Check if token mint is provided
	if task.TokenMint == "" {
		return fmt.Errorf("token mint address is required for Pump.Swap")
	}

	tokenMint := task.TokenMint

	// Initialize DEX if not already done
	if d.inner == nil {
		if err := d.initializeInnerDEX(ctx, tokenMint); err != nil {
			return err
		}
	}

	// Execute operation based on task type
	switch task.Operation {
	case OperationSwap:
		d.logger.Info("Executing swap on Pump.Swap",
			zap.String("token_mint", tokenMint),
			zap.Float64("amount_sol", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		// Convert SOL amount to lamports for swap
		amountLamports := uint64(task.AmountSol * 1e9)

		// Execute swap (buy) operation
		return d.inner.ExecuteSwap(ctx, pumpswap.SwapParams{
			IsBuy:           true,
			Amount:          amountLamports,
			SlippagePercent: task.SlippagePercent,
			PriorityFeeSol:  task.PriorityFee,
			ComputeUnits:    task.ComputeUnits,
		})

	case OperationSell:
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

		return d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	default:
		return fmt.Errorf("operation %s is not supported on Pump.Swap", task.Operation)
	}
}

// initializeInnerDEX инициализирует внутренний DEX с заданным токеном
func (d *pumpswapDEXAdapter) initializeInnerDEX(ctx context.Context, tokenMint string) error {
	config := pumpswap.GetDefaultConfig()
	if err := config.SetupForToken(tokenMint, d.logger); err != nil {
		return fmt.Errorf("failed to setup token config: %w", err)
	}

	poolManager := pumpswap.NewPoolManager(d.client, d.logger)

	var dexErr error
	d.inner, dexErr = pumpswap.NewDEX(d.client, d.wallet, d.logger, config, poolManager, config.MonitorInterval)
	if dexErr != nil {
		return fmt.Errorf("failed to initialize Pump.Swap DEX: %w", dexErr)
	}

	return nil
}

// GetTokenPrice returns the current price of the token
func (d *pumpswapDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Инициализировать DEX если необходимо
	if d.inner == nil {
		if err := d.initializeInnerDEX(ctx, tokenMint); err != nil {
			return 0, err
		}
	}

	// Делегируем вызов внутренней реализации
	return d.inner.GetTokenPrice(ctx, tokenMint)
}
