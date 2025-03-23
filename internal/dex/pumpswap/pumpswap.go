// =============================
// File: internal/dex/pumpswap/pumpswap.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// DEX implements the PumpSwap DEX interface
type DEX struct {
	client          *solbc.Client
	wallet          *wallet.Wallet
	logger          *zap.Logger
	config          *Config
	poolManager     *PoolManager
	priorityManager *types.PriorityManager
}

// NewDEX creates a new PumpSwap DEX instance
func NewDEX(
	client *solbc.Client,
	w *wallet.Wallet,
	logger *zap.Logger,
	config *Config,
	monitorInterval string,
) (*DEX, error) {
	// Validate client and wallet
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Set monitor interval if provided
	if monitorInterval != "" {
		config.MonitorInterval = monitorInterval
	}

	// Create pool manager
	poolManager := NewPoolManager(client, logger)

	// Create priority manager
	priorityManager := types.NewPriorityManager(logger)

	dex := &DEX{
		client:          client,
		wallet:          w,
		logger:          logger,
		config:          config,
		poolManager:     poolManager,
		priorityManager: priorityManager,
	}

	return dex, nil
}

// findAndValidatePool находит и проверяет пул для обмена
func (dex *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	// Поиск пула
	pool, err := dex.poolManager.FindPoolWithRetry(
		ctx,
		dex.config.BaseMint,
		dex.config.QuoteMint,
		5,             // max retries
		time.Second*2, // retry delay
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	// Обновляем конфигурацию
	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	// Логируем детали найденного пула
	dex.logger.Debug("Found pool details",
		zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	// Проверяем, совпадает ли порядок монет в пуле с ожидаемым
	poolMintOrderReversed := !pool.BaseMint.Equals(dex.config.BaseMint)

	return pool, poolMintOrderReversed, nil
}

// prepareTokenAccounts подготавливает все необходимые токеновые аккаунты
func (dex *DEX) prepareTokenAccounts(ctx context.Context, pool *PoolInfo) (
	userBaseATA, userQuoteATA, protocolFeeRecipientATA solana.PublicKey,
	protocolFeeRecipient solana.PublicKey,
	createBaseATAIx, createQuoteATAIx solana.Instruction,
	err error) {

	// Получаем адреса ATA пользователя
	userBaseATA, _, err = solana.FindAssociatedTokenAddress(dex.wallet.PublicKey, pool.BaseMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, nil, nil,
			fmt.Errorf("failed to derive user base mint ATA: %w", err)
	}

	userQuoteATA, _, err = solana.FindAssociatedTokenAddress(dex.wallet.PublicKey, pool.QuoteMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, nil, nil,
			fmt.Errorf("failed to derive user quote mint ATA: %w", err)
	}

	// Создаем инструкции для идемпотентного создания ATA
	createBaseATAIx = dex.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		dex.wallet.PublicKey, dex.wallet.PublicKey, pool.BaseMint)

	createQuoteATAIx = dex.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		dex.wallet.PublicKey, dex.wallet.PublicKey, pool.QuoteMint)

	// Получаем информацию о глобальной конфигурации
	globalConfigAddr, _, err := dex.config.DeriveGlobalConfigAddress()
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, nil, nil,
			fmt.Errorf("failed to derive global config address: %w", err)
	}

	globalConfigInfo, err := dex.client.GetAccountInfo(ctx, globalConfigAddr)
	if err != nil || globalConfigInfo == nil || globalConfigInfo.Value == nil {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, nil, nil,
			fmt.Errorf("failed to get global config: %w", err)
	}

	globalConfig, err := ParseGlobalConfig(globalConfigInfo.Value.Data.GetBinary())
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, nil, nil,
			fmt.Errorf("failed to parse global config: %w", err)
	}

	// Получаем получателя комиссии и его ATA
	protocolFeeRecipient = solana.PublicKeyFromBytes(make([]byte, 32))
	if len(globalConfig.ProtocolFeeRecipients) > 0 {
		protocolFeeRecipient = globalConfig.ProtocolFeeRecipients[0]
	}

	protocolFeeRecipientATA, _, err = solana.FindAssociatedTokenAddress(
		protocolFeeRecipient,
		pool.QuoteMint,
	)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, nil, nil,
			fmt.Errorf("failed to derive protocol fee recipient ATA: %w", err)
	}

	return userBaseATA, userQuoteATA, protocolFeeRecipientATA, protocolFeeRecipient, createBaseATAIx, createQuoteATAIx, nil
}

// preparePriorityInstructions подготавливает инструкции для установки вычислительных единиц и приоритета
func (dex *DEX) preparePriorityInstructions(computeUnits uint32, priorityFeeSol string) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// Добавляем инструкции для установки вычислительных единиц
	if computeUnits > 0 {
		instructions = append(instructions,
			computebudget.NewSetComputeUnitLimitInstruction(computeUnits).Build())
	}

	// Устанавливаем приоритет, если указан
	if priorityFeeSol != "" {
		priorityFeeFloat, err := strconv.ParseFloat(priorityFeeSol, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid priority fee format: %w", err)
		}

		// Convert SOL to micro-lamports (1 lamport = 10^-9 SOL, 1 micro-lamport = 10^-6 lamport)
		priorityFeeMicroLamports := uint64(priorityFeeFloat * 1e9 * 1e6)
		if priorityFeeMicroLamports > 0 {
			instructions = append(instructions,
				computebudget.NewSetComputeUnitPriceInstruction(priorityFeeMicroLamports).Build())
		}
	}

	return instructions, nil
}

// calculateSwapAmounts рассчитывает минимальный ожидаемый вывод с учетом слиппажа
func (dex *DEX) calculateSwapAmounts(pool *PoolInfo, amount uint64, isBuy bool, slippagePercent float64) uint64 {
	// Рассчитываем ожидаемый и минимальный вывод с учетом слиппажа
	outputAmount, _ := dex.poolManager.CalculateSwapQuote(pool, amount, isBuy)
	minOutAmount := uint64(float64(outputAmount) * (1.0 - slippagePercent/100.0))

	return minOutAmount
}

// buildAndSubmitTransaction создает, подписывает и отправляет транзакцию, ожидая подтверждения
func (dex *DEX) buildAndSubmitTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	// Получаем последний блокхэш
	recentBlockhash, err := dex.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем транзакцию
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockhash,
		solana.TransactionPayer(dex.wallet.PublicKey),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию
	if err := dex.wallet.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Отправляем транзакцию
	signature, err := dex.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	// Ожидаем подтверждения
	err = dex.client.WaitForTransactionConfirmation(ctx, signature, rpc.CommitmentConfirmed)
	if err != nil {
		return signature, fmt.Errorf("transaction failed: %w", err)
	}

	return signature, nil
}

// ExecuteSwap executes a swap operation
func (dex *DEX) ExecuteSwap(
	ctx context.Context,
	isBuy bool,
	amount uint64,
	slippagePercent float64,
	priorityFeeSol string,
	computeUnits uint32,
) error {
	// 1. Поиск и валидация пула
	pool, poolMintOrderReversed, err := dex.findAndValidatePool(ctx)
	if err != nil {
		return err
	}

	// Определяем фактическое направление обмена с учетом порядка токенов в пуле
	actualIsBuy := isBuy
	if poolMintOrderReversed {
		actualIsBuy = !isBuy
		dex.logger.Debug("Reversing buy/sell operation due to pool mint order",
			zap.Bool("actual_is_buy", actualIsBuy))
	}

	// 2. Подготовка токеновых аккаунтов и ATA инструкций
	userBaseATA, userQuoteATA, protocolFeeRecipientATA, protocolFeeRecipient,
		createBaseATAIx, createQuoteATAIx, err := dex.prepareTokenAccounts(ctx, pool)
	if err != nil {
		return err
	}

	// 3. Подготовка инструкций приоритета
	priorityInstructions, err := dex.preparePriorityInstructions(computeUnits, priorityFeeSol)
	if err != nil {
		return err
	}

	// Собираем все инструкции
	var instructions []solana.Instruction
	instructions = append(instructions, priorityInstructions...)
	instructions = append(instructions, createBaseATAIx, createQuoteATAIx)

	// 4. Расчет минимального вывода с учетом слиппажа и создание инструкции свопа
	minOutAmount := dex.calculateSwapAmounts(pool, amount, actualIsBuy, slippagePercent)

	// Создаем инструкцию свопа в зависимости от направления
	var swapInstruction solana.Instruction
	if actualIsBuy {
		swapInstruction = createBuyInstruction(
			pool.Address,
			dex.wallet.PublicKey,
			dex.config.GlobalConfig,
			pool.BaseMint,
			pool.QuoteMint,
			userBaseATA,
			userQuoteATA,
			pool.PoolBaseTokenAccount,
			pool.PoolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientATA,
			TokenProgramID,
			TokenProgramID,
			dex.config.EventAuthority,
			dex.config.ProgramID,
			amount,
			minOutAmount,
		)
	} else {
		swapInstruction = createSellInstruction(
			pool.Address,
			dex.wallet.PublicKey,
			dex.config.GlobalConfig,
			pool.BaseMint,
			pool.QuoteMint,
			userBaseATA,
			userQuoteATA,
			pool.PoolBaseTokenAccount,
			pool.PoolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientATA,
			TokenProgramID,
			TokenProgramID,
			dex.config.EventAuthority,
			dex.config.ProgramID,
			amount,
			minOutAmount,
		)
	}

	instructions = append(instructions, swapInstruction)

	// 5. Отправка транзакции и ожидание подтверждения
	signature, err := dex.buildAndSubmitTransaction(ctx, instructions)
	if err != nil {
		return err
	}

	dex.logger.Info("Swap transaction confirmed",
		zap.String("signature", signature.String()),
		zap.Bool("is_buy", isBuy),
		zap.Bool("actual_is_buy", actualIsBuy),
		zap.Uint64("amount", amount),
		zap.Float64("slippage_percent", slippagePercent))

	return nil
}

// ExecuteSell implements the sell operation for PumpSwap
func (dex *DEX) ExecuteSell(
	ctx context.Context,
	tokenAmount uint64,
	slippagePercent float64,
	priorityFeeSol string,
	computeUnits uint32,
) error {
	// Execute sell operation
	return dex.ExecuteSwap(ctx, false, tokenAmount, slippagePercent, priorityFeeSol, computeUnits)
}

// GetTokenPrice retrieves the current price of the token
func (dex *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Validate token mint matches config
	mint, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint address: %w", err)
	}

	// Make sure the token mint matches our config
	if !mint.Equals(dex.config.QuoteMint) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s",
			dex.config.QuoteMint.String(), mint.String())
	}

	// Find the pool and get pool info
	pool, err := dex.poolManager.FindPoolWithRetry(
		ctx,
		dex.config.BaseMint,
		dex.config.QuoteMint,
		3,             // max retries
		time.Second*1, // retry delay
	)
	if err != nil {
		return 0, fmt.Errorf("failed to find pool: %w", err)
	}

	// Calculate the price based on pool reserves
	// For SOL/TOKEN pair, price is SOL per TOKEN
	var price float64
	if pool.BaseReserves > 0 && pool.QuoteReserves > 0 {
		// Get the decimal precision for both tokens
		solDecimals := uint8(9) // SOL has 9 decimals
		tokenDecimals, err := dex.DetermineTokenPrecision(ctx, dex.config.QuoteMint)
		if err != nil {
			// Default to 6 decimals if cannot determine
			tokenDecimals = 6
			dex.logger.Warn("Could not determine token precision, using default",
				zap.Uint8("default_decimals", tokenDecimals),
				zap.Error(err))
		}

		// Adjust reserves based on token decimals
		baseReservesFloat := new(big.Float).SetUint64(pool.BaseReserves)
		quoteReservesFloat := new(big.Float).SetUint64(pool.QuoteReserves)

		// Calculate the price: base_reserves / quote_reserves, adjusted for decimals
		// Price = (base_reserves / 10^base_decimals) / (quote_reserves / 10^quote_decimals)
		//       = (base_reserves * 10^quote_decimals) / (quote_reserves * 10^base_decimals)
		baseAdjustment := math.Pow10(int(solDecimals))
		quoteAdjustment := math.Pow10(int(tokenDecimals))

		// Perform calculation: price = (base_reserves / quote_reserves) * (10^quote_decimals / 10^base_decimals)
		ratio := new(big.Float).Quo(baseReservesFloat, quoteReservesFloat)
		decimalAdjustment := float64(quoteAdjustment) / float64(baseAdjustment)

		adjustedRatio := new(big.Float).Mul(ratio, big.NewFloat(decimalAdjustment))
		price, _ = adjustedRatio.Float64()
	}

	return price, nil
}

// DetermineTokenPrecision gets the decimal precision for a token
func (dex *DEX) DetermineTokenPrecision(ctx context.Context, mint solana.PublicKey) (uint8, error) {
	// Get the mint account info
	mintInfo, err := dex.client.GetAccountInfo(ctx, mint)
	if err != nil {
		return 0, fmt.Errorf("failed to get mint info: %w", err)
	}

	if mintInfo == nil || mintInfo.Value == nil {
		return 0, fmt.Errorf("mint account not found")
	}

	// SPL Token mint account layout has decimals at offset 44 (1 byte)
	data := mintInfo.Value.Data.GetBinary()
	if len(data) < 45 {
		return 0, fmt.Errorf("mint account data too short")
	}

	// Extract decimals (1 byte at offset 44)
	decimals := data[44]

	return decimals, nil
}
