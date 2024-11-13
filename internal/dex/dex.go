// internal/dex/dex.go
package dex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// GetDEXByName возвращает имплементацию DEX по имени
func GetDEXByName(name string, client blockchain.Client, logger *zap.Logger) (types.DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return nil, fmt.Errorf("DEX name cannot be empty")
	}

	logger = logger.With(zap.String("dex_name", name))
	logger.Info("Initializing DEX instance")

	switch name {
	case "raydium":
		return initializeRaydiumDEX(client, logger)
	case "pump.fun":
		return nil /*initializePumpFunDEX(client, logger)*/, nil
	default:
		logger.Error("Unsupported DEX requested", zap.String("name", name))
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}

// initializeRaydiumDEX инициализирует Raydium DEX
func initializeRaydiumDEX(client blockchain.Client, logger *zap.Logger) (types.DEX, error) {
	solClient, ok := client.(*solbc.Client)
	if !ok {
		return nil, fmt.Errorf("invalid client type")
	}

	// Получаем RPC endpoint
	endpoint := solClient.GetRPCEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("empty RPC endpoint")
	}

	// Получаем приватный ключ
	walletKey, err := solClient.GetWalletKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet key: %w", err)
	}

	// Создаем Raydium клиент
	raydiumClient, err := raydium.NewRaydiumClient(endpoint, walletKey, logger.Named("raydium"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Raydium client: %w", err)
	}

	// Создаем конфигурацию
	config := &raydium.SniperConfig{
		MaxSlippageBps:   500,        // 5%
		MinAmountSOL:     100000,     // 0.0001 SOL
		MaxAmountSOL:     1000000000, // 1 SOL
		PriorityFee:      1000,
		WaitConfirmation: true,
		MonitorInterval:  time.Second,
		MaxRetries:       3,
	}

	return &raydiumDEX{
		client: raydiumClient,
		logger: logger,
		config: config,
	}, nil
}

// // initializePumpFunDEX инициализирует Pump.fun DEX
// func initializePumpFunDEX(_ blockchain.Client, logger *zap.Logger) (types.DEX, error) {
// 	logger.Debug("Initializing Pump.fun DEX")

// 	// Создаем новый экземпляр Pump.fun DEX
// 	dex := pumpfun.NewDEX()
// 	if dex == nil {
// 		logger.Error("Failed to create Pump.fun DEX instance")
// 		return nil, fmt.Errorf("failed to create Pump.fun DEX instance")
// 	}

// 	logger.Info("Pump.fun DEX initialized successfully")
// 	return dex, nil
// }

// Улучшить имплементацию raydiumDEX:

// validateAndPrepareSwap подготавливает и проверяет все необходимые параметры для выполнения свапа
func (d *raydiumDEX) validateAndPrepareSwap(ctx context.Context, task *types.Task, w *wallet.Wallet) (*raydium.SwapParams, error) {
	logger := d.logger.With(
		zap.String("source_token", task.SourceToken),
		zap.String("target_token", task.TargetToken),
		zap.Float64("amount_in", task.AmountIn),
	)
	logger.Debug("starting swap validation and preparation")

	// 1. Валидация параметров задачи
	if err := d.validateTaskParameters(task, w); err != nil {
		return nil, fmt.Errorf("invalid task parameters: %w", err)
	}

	// 2. Конвертация публичных ключей
	sourceMint, err := solana.PublicKeyFromBase58(task.SourceToken)
	if err != nil {
		return nil, fmt.Errorf("invalid source token address: %w", err)
	}
	targetMint, err := solana.PublicKeyFromBase58(task.TargetToken)
	if err != nil {
		return nil, fmt.Errorf("invalid target token address: %w", err)
	}

	// 3. Поиск лучшего пула
	pool, err := d.client.GetPool(ctx, sourceMint, targetMint)
	if err != nil {
		return nil, fmt.Errorf("failed to find suitable pool: %w", err)
	}
	logger.Debug("found suitable pool",
		zap.String("pool_id", pool.ID.String()),
		zap.Uint64("base_reserve", pool.State.BaseReserve),
		zap.Uint64("quote_reserve", pool.State.QuoteReserve))

	// 4. Проверка состояния пула
	if err := d.validatePoolState(pool); err != nil {
		return nil, fmt.Errorf("pool validation failed: %w", err)
	}

	// 5. Конвертация amount в лампорты
	amountInLamports := uint64(task.AmountIn * float64(solana.LAMPORTS_PER_SOL))
	priorityFeeLamports := uint64(task.PriorityFee * float64(solana.LAMPORTS_PER_SOL))

	// 6. Расчет выходного количества с учетом слиппажа
	amounts := raydium.CalculateSwapAmounts(pool, amountInLamports, d.config.MaxSlippageBps)
	logger.Debug("calculated swap amounts",
		zap.Uint64("amount_in", amounts.AmountIn),
		zap.Uint64("amount_out", amounts.AmountOut),
		zap.Uint64("min_amount_out", amounts.MinAmountOut))

	// 7. Проверка влияния на цену
	priceImpact := raydium.GetPriceImpact(pool, amounts.AmountIn)
	if priceImpact > float64(d.config.MaxSlippageBps)/100.0 {
		return nil, fmt.Errorf("price impact too high: %.2f%%", priceImpact)
	}

	// 8. Проверка баланса
	if err := d.validateBalance(ctx, w.PublicKey, amounts.AmountIn, priorityFeeLamports); err != nil {
		return nil, fmt.Errorf("insufficient balance: %w", err)
	}

	// 9. Создание параметров свапа
	swapParams := &raydium.SwapParams{
		UserWallet:          w.PublicKey,
		PrivateKey:          &w.PrivateKey,
		AmountIn:            amounts.AmountIn,
		MinAmountOut:        amounts.MinAmountOut,
		Pool:                pool,
		PriorityFeeLamports: priorityFeeLamports,
		Direction:           raydium.SwapDirectionIn,
		SlippageBps:         d.config.MaxSlippageBps,
		WaitConfirmation:    d.config.WaitConfirmation,
	}

	logger.Info("swap parameters prepared successfully",
		zap.Uint64("amount_in", swapParams.AmountIn),
		zap.Uint64("min_amount_out", swapParams.MinAmountOut),
		zap.Float64("price_impact", priceImpact))

	return swapParams, nil
}

// validateTaskParameters проверяет корректность параметров задачи
func (d *raydiumDEX) validateTaskParameters(task *types.Task, w *wallet.Wallet) error {
	if task == nil {
		return fmt.Errorf("task cannot be nil")
	}
	if w == nil {
		return fmt.Errorf("wallet cannot be nil")
	}
	if task.SourceToken == "" || task.TargetToken == "" {
		return fmt.Errorf("invalid token addresses")
	}
	if task.AmountIn <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if task.PriorityFee < 0 {
		return fmt.Errorf("priority fee cannot be negative")
	}
	return nil
}

// validatePoolState проверяет состояние пула
func (d *raydiumDEX) validatePoolState(pool *raydium.Pool) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}
	if pool.State.Status != raydium.PoolStatusActive {
		return fmt.Errorf("pool is not active")
	}
	if pool.State.BaseReserve == 0 || pool.State.QuoteReserve == 0 {
		return fmt.Errorf("pool has no liquidity")
	}
	return nil
}

// validateBalance проверяет достаточность баланса
func (d *raydiumDEX) validateBalance(ctx context.Context, wallet solana.PublicKey, amountIn, priorityFee uint64) error {
	balance, err := d.client.GetBaseClient().GetBalance(
		ctx,
		wallet,
		rpc.CommitmentConfirmed, // Используем подтвержденный уровень
	)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	requiredBalance := amountIn + priorityFee + 5000 // 5000 lamports для комиссии сети
	if balance < requiredBalance {
		return fmt.Errorf("insufficient balance: required %d, got %d", requiredBalance, balance)
	}
	return nil
}
