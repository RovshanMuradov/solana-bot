// package pumpfun provides integration with the Pump.fun protocol on Solana
package pumpfun

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX is the Pump.fun DEX implementation
type DEX struct {
	client *solbc.Client
	wallet *wallet.Wallet
	logger *zap.Logger
	config *Config
}

// NewDEX creates a new instance of the Pump.fun DEX
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, _ string) (*DEX, error) {
	if config.ContractAddress.IsZero() {
		return nil, fmt.Errorf("pump.fun contract address is required")
	}
	if config.Mint.IsZero() {
		return nil, fmt.Errorf("token mint address is required")
	}

	logger.Info("Creating PumpFun DEX",
		zap.String("contract", config.ContractAddress.String()),
		zap.String("token_mint", config.Mint.String()))

	// Create DEX instance
	dex := &DEX{
		client: client,
		wallet: w,
		logger: logger.Named("pumpfun"),
		config: config,
	}

	// Update fee recipient
	fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	globalAccount, err := FetchGlobalAccount(fetchCtx, client, config.Global)
	if err != nil {
		logger.Warn("Failed to fetch global account data, using default fee recipient",
			zap.Error(err))
	} else if globalAccount != nil {
		// Update fee recipient from global account data
		config.FeeRecipient = globalAccount.FeeRecipient
		logger.Info("Updated fee recipient", zap.String("address", config.FeeRecipient.String()))
	}

	return dex, nil
}

// ExecuteSnipe executes a buy operation on the Pump.fun protocol
func (d *DEX) ExecuteSnipe(ctx context.Context, amount, maxSolCost uint64) error {
	d.logger.Info("Starting Pump.fun buy operation",
		zap.Uint64("amount", amount),
		zap.Uint64("max_sol_cost", maxSolCost))

	// Create a context with timeout for the entire operation
	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	// Get latest blockhash
	blockhash, err := d.client.GetRecentBlockhash(opCtx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	d.logger.Debug("Got blockhash", zap.String("blockhash", blockhash.String()))

	// Create instruction #1: Set Compute Unit Limit (61,368 compute units)
	instr1 := createSetComputeUnitLimitInstruction(61368)

	// Instruction #2: Set Compute Unit Price (0.1 lamports per compute unit)
	instr2 := createSetComputeUnitPriceInstruction(100)

	// Instruction #3: Create Associated Token Account Idempotent
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated token account: %w", err)
	}
	instr3 := createAssociatedTokenAccountIdempotentInstruction(d.wallet.PublicKey, d.wallet.PublicKey, d.config.Mint)

	// Ensure bonding curve is derived correctly
	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to derive bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve", zap.String("address", bondingCurve.String()))

	// Calculate associated bonding curve ATA
	associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(bondingCurve, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve ATA", zap.String("address", associatedBondingCurve.String()))

	// Instruction #4: buy
	buyIx := createBuyInstruction(
		d.config.ContractAddress, // Program ID
		d.config.Global,          // Global account
		d.config.FeeRecipient,    // Fee recipient
		d.config.Mint,            // Token mint
		bondingCurve,             // Bonding curve
		associatedBondingCurve,   // Associated bonding curve ATA
		userATA,                  // User's associated token account
		d.wallet.PublicKey,       // User's wallet
		d.config.EventAuthority,  // Event authority
		amount,                   // Amount of tokens to buy
		maxSolCost,               // Maximum SOL cost
	)

	// Assemble all instructions
	instructions := []solana.Instruction{instr1, instr2, instr3, buyIx}

	// Create transaction
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	if err := d.wallet.SignTransaction(tx); err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Simulate transaction
	simResult, err := d.client.SimulateTransaction(opCtx, tx)
	if err != nil || (simResult != nil && simResult.Err != nil) {
		d.logger.Warn("Transaction simulation failed",
			zap.Error(err),
			zap.Any("sim_error", simResult != nil && simResult.Err != nil))

		// Continue anyway as simulation can sometimes fail for valid transactions
	} else {
		d.logger.Info("Transaction simulation successful",
			zap.Uint64("compute_units", simResult.UnitsConsumed))
	}

	// Send transaction
	txSig, err := d.client.SendTransaction(opCtx, tx)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	d.logger.Info("Buy transaction sent successfully",
		zap.String("signature", txSig.String()))

	// Wait for confirmation
	if err := d.client.WaitForTransactionConfirmation(opCtx, txSig, rpc.CommitmentConfirmed); err != nil {
		d.logger.Warn("Failed to confirm transaction",
			zap.String("signature", txSig.String()),
			zap.Error(err))
		return fmt.Errorf("transaction confirmation failed: %w", err)
	}

	d.logger.Info("Buy transaction confirmed",
		zap.String("signature", txSig.String()))
	return nil
}

// ExecuteSell executes a sell operation on the Pump.fun protocol
func (d *DEX) ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error {
	d.logger.Info("Starting Pump.fun sell operation",
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput))

	// Create a context with timeout for the entire operation
	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	// Get latest blockhash
	blockhash, err := d.client.GetRecentBlockhash(opCtx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	d.logger.Debug("Got blockhash", zap.String("blockhash", blockhash.String()))

	// Create instruction #1: Set Compute Unit Limit (61,368 compute units)
	instr1 := createSetComputeUnitLimitInstruction(61368)

	// Instruction #2: Set Compute Unit Price (0.1 lamports per compute unit)
	instr2 := createSetComputeUnitPriceInstruction(100)

	// Instruction #3: Create Associated Token Account Idempotent
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated token account: %w", err)
	}
	instr3 := createAssociatedTokenAccountIdempotentInstruction(d.wallet.PublicKey, d.wallet.PublicKey, d.config.Mint)

	// Ensure bonding curve is derived correctly
	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to derive bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve", zap.String("address", bondingCurve.String()))

	// Calculate associated bonding curve ATA
	associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(bondingCurve, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve ATA", zap.String("address", associatedBondingCurve.String()))

	// Instruction #4: sell
	sellIx := createSellInstruction(
		d.config.ContractAddress, // Program ID
		d.config.Global,          // Global account
		d.config.FeeRecipient,    // Fee recipient
		d.config.Mint,            // Token mint
		bondingCurve,             // Bonding curve
		associatedBondingCurve,   // Associated bonding curve ATA
		userATA,                  // User's associated token account
		d.wallet.PublicKey,       // User's wallet
		d.config.EventAuthority,  // Event authority
		amount,                   // Amount of tokens to sell
		minSolOutput,             // Minimum SOL output
	)

	// Assemble all instructions
	instructions := []solana.Instruction{instr1, instr2, instr3, sellIx}

	// Create transaction
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	if err := d.wallet.SignTransaction(tx); err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Simulate transaction
	simResult, err := d.client.SimulateTransaction(opCtx, tx)
	if err != nil || (simResult != nil && simResult.Err != nil) {
		d.logger.Warn("Transaction simulation failed",
			zap.Error(err),
			zap.Any("sim_error", simResult != nil && simResult.Err != nil))

		// Continue anyway as simulation can sometimes fail for valid transactions
	} else {
		d.logger.Info("Transaction simulation successful",
			zap.Uint64("compute_units", simResult.UnitsConsumed))
	}

	// Send transaction
	txSig, err := d.client.SendTransaction(opCtx, tx)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	d.logger.Info("Sell transaction sent successfully",
		zap.String("signature", txSig.String()))

	// Wait for confirmation
	if err := d.client.WaitForTransactionConfirmation(opCtx, txSig, rpc.CommitmentConfirmed); err != nil {
		d.logger.Warn("Failed to confirm transaction",
			zap.String("signature", txSig.String()),
			zap.Error(err))
		return fmt.Errorf("transaction confirmation failed: %w", err)
	}

	d.logger.Info("Sell transaction confirmed",
		zap.String("signature", txSig.String()))
	return nil
}
