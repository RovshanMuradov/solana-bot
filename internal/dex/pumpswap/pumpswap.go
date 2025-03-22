package pumpswap

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
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

// ExecuteSwap executes a swap operation
func (dex *DEX) ExecuteSwap(
	ctx context.Context,
	isBuy bool,
	amount uint64,
	slippagePercent float64,
	priorityFeeSol string,
	computeUnits uint32,
) error {
	// Find the pool for the token pair
	pool, err := dex.poolManager.FindPoolWithRetry(
		ctx,
		dex.config.BaseMint,
		dex.config.QuoteMint,
		5, // max retries
		time.Second*2, // retry delay
	)
	if err != nil {
		return fmt.Errorf("failed to find pool: %w", err)
	}

	// Update config with found pool address
	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	// Get the user's token accounts
	userSolATA, err := dex.wallet.GetATA(dex.config.BaseMint)
	if err != nil {
		return fmt.Errorf("failed to get user SOL ATA: %w", err)
	}

	userTokenATA, err := dex.wallet.GetATA(dex.config.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to get user token ATA: %w", err)
	}

	// Get protocol fee recipient from the first one in the global config
	// TODO: fetch this from the global config account
	protocolFeeRecipient := solana.PublicKeyFromBytes(make([]byte, 32))
	protocolFeeRecipientATA, _, err := solana.FindAssociatedTokenAddress(
		protocolFeeRecipient,
		dex.config.QuoteMint,
	)
	if err != nil {
		return fmt.Errorf("failed to derive protocol fee recipient ATA: %w", err)
	}

	// Fetch Global Config to get the current protocol fee recipient
	globalConfigAddr, _, err := dex.config.DeriveGlobalConfigAddress()
	if err != nil {
		return fmt.Errorf("failed to derive global config address: %w", err)
	}

	globalConfigInfo, err := dex.client.GetAccountInfo(ctx, globalConfigAddr)
	if err != nil || globalConfigInfo == nil || globalConfigInfo.Value == nil {
		return fmt.Errorf("failed to get global config: %w", err)
	}

	globalConfig, err := ParseGlobalConfig(globalConfigInfo.Value.Data.GetBinary())
	if err != nil {
		return fmt.Errorf("failed to parse global config: %w", err)
	}

	// Use the first protocol fee recipient (if available)
	if len(globalConfig.ProtocolFeeRecipients) > 0 {
		protocolFeeRecipient = globalConfig.ProtocolFeeRecipients[0]
		// Re-derive ATA with the correct protocol fee recipient
		protocolFeeRecipientATA, _, err = solana.FindAssociatedTokenAddress(
			protocolFeeRecipient,
			dex.config.QuoteMint,
		)
		if err != nil {
			return fmt.Errorf("failed to derive protocol fee recipient ATA: %w", err)
		}
	}

	// Get the recent blockhash
	recentBlockhash, err := dex.client.GetRecentBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Create a list of instructions
	var instructions []solana.Instruction

	// Add compute budget instructions if needed
	if computeUnits > 0 {
		instructions = append(instructions, createComputeBudgetRequestUnitsInstruction(computeUnits))
	}

	// Parse priority fee if provided
	if priorityFeeSol != "" {
		priorityFeeFloat, err := strconv.ParseFloat(priorityFeeSol, 64)
		if err != nil {
			return fmt.Errorf("invalid priority fee format: %w", err)
		}

		// Convert SOL to micro-lamports (1 lamport = 10^-9 SOL, 1 micro-lamport = 10^-6 lamport)
		priorityFeeMicroLamports := uint64(priorityFeeFloat * 1e9 * 1e6)
		if priorityFeeMicroLamports > 0 {
			instructions = append(instructions, createComputeBudgetRequestUnitsFeeInstruction(0, priorityFeeMicroLamports))
		}
	}

	// Calculate expected output
	var minOutAmount uint64

	if isBuy {
		// Buying token with SOL
		outputAmount, _ := dex.poolManager.CalculateSwapQuote(pool, amount, true)
		minOutAmount = uint64(float64(outputAmount) * (1.0 - slippagePercent/100.0))

		// Create the buy instruction
		buyInstruction := createBuyInstruction(
			pool.Address,
			dex.wallet.PublicKey,
			dex.config.GlobalConfig,
			dex.config.BaseMint,
			dex.config.QuoteMint,
			userSolATA,
			userTokenATA,
			pool.PoolBaseTokenAccount,
			pool.PoolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientATA,
			TokenProgramID,  // Base token program (SOL)
			TokenProgramID,  // Quote token program
			dex.config.EventAuthority,
			dex.config.ProgramID,
			amount,          // baseAmountOut (amount of SOL to spend)
			minOutAmount,    // maxQuoteAmountIn (min tokens to receive with slippage)
		)

		instructions = append(instructions, buyInstruction)
	} else {
		// Selling token for SOL
		outputAmount, _ := dex.poolManager.CalculateSwapQuote(pool, amount, false)
		minOutAmount = uint64(float64(outputAmount) * (1.0 - slippagePercent/100.0))

		// Create the sell instruction
		sellInstruction := createSellInstruction(
			pool.Address,
			dex.wallet.PublicKey,
			dex.config.GlobalConfig,
			dex.config.BaseMint,
			dex.config.QuoteMint,
			userSolATA,
			userTokenATA,
			pool.PoolBaseTokenAccount,
			pool.PoolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientATA,
			TokenProgramID,  // Base token program (SOL)
			TokenProgramID,  // Quote token program
			dex.config.EventAuthority,
			dex.config.ProgramID,
			amount,          // baseAmountIn (amount of tokens to sell)
			minOutAmount,    // minQuoteAmountOut (min SOL to receive with slippage)
		)

		instructions = append(instructions, sellInstruction)
	}

	// Create transaction with all instructions
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockhash,
		solana.TransactionPayer(dex.wallet.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	if err := dex.wallet.SignTransaction(tx); err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction
	signature, err := dex.client.SendTransaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	dex.logger.Info("Swap transaction sent",
		zap.String("signature", signature.String()),
		zap.Bool("is_buy", isBuy),
		zap.Uint64("amount", amount),
		zap.Float64("slippage_percent", slippagePercent))

	// Wait for confirmation
	err = dex.client.WaitForTransactionConfirmation(ctx, signature, rpc.CommitmentConfirmed)
	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	dex.logger.Info("Swap transaction confirmed",
		zap.String("signature", signature.String()))

	return nil
}

// ExecuteSnipe implements the snipe operation for PumpSwap
func (dex *DEX) ExecuteSnipe(
	ctx context.Context,
	amountSol float64,
	slippagePercent float64,
	priorityFeeSol string,
	computeUnits uint32,
) error {
	// Convert SOL amount to lamports
	amountLamports := uint64(amountSol * 1e9)

	// Execute buy operation
	return dex.ExecuteSwap(ctx, true, amountLamports, slippagePercent, priorityFeeSol, computeUnits)
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
		3, // max retries
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