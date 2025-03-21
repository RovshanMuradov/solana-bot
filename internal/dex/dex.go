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

	case OperationSnipeMonitor:
		// This is handled at a higher level by combining Snipe and Monitor operations
		// Here we just execute the snipe part
		txType = "snipe_monitor"

		d.logger.Info("Executing snipe (in snipe_monitor mode) on Pump.fun",
			zap.String("token_mint", tokenMint),
			zap.Float64("amount_sol", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits),
			zap.Duration("monitor_interval", task.MonitorInterval))

		err := d.inner.ExecuteSnipe(ctx, task.AmountSol, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
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

		// Convert SOL amount to lamports
		amountInLamports := uint64(task.AmountSol * 1_000_000_000)

		// Calculate min output based on slippage
		minOutLamports := uint64(task.AmountSol * (1 - task.SlippagePercent/100.0) * 1_000_000_000)

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
			AmountInLamports:    amountInLamports,
			MinOutLamports:      minOutLamports,
			PriorityFeeLamports: 0, // Это нужно обновить используя task.PriorityFee
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

// GetTokenPrice returns token price for Raydium DEX
func (d *raydiumDEXAdapter) GetTokenPrice(_ context.Context, _ string) (float64, error) {
	return 0, fmt.Errorf("price retrieval not implemented for Raydium DEX")
}
