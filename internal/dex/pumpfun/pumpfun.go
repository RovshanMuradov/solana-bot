// ==============================================
// File: internal/dex/pumpfun/pumpfun.go
// ==============================================

package pumpfun

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX is the Pump.fun DEX implementation.
type DEX struct {
	client        *solbc.Client
	wallet        *wallet.Wallet
	logger        *zap.Logger
	config        *Config
	errorAnalyzer *solbc.ErrorAnalyzer
	stateChecker  *ProgramStateChecker
}

// NewDEX creates a new instance of DEX.
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, _ string) (*DEX, error) {
	// Validate required configuration
	if config.ContractAddress.IsZero() {
		return nil, fmt.Errorf("pump.fun contract address is required")
	}
	if config.Mint.IsZero() {
		return nil, fmt.Errorf("token mint address is required")
	}
	if config.BondingCurve.IsZero() {
		return nil, fmt.Errorf("bonding curve address is required")
	}

	logger.Info("Creating PumpFun DEX",
		zap.String("contract", config.ContractAddress.String()),
		zap.String("token_mint", config.Mint.String()),
		zap.String("bonding_curve", config.BondingCurve.String()))

	// Create error analyzer
	errorAnalyzer := solbc.NewErrorAnalyzer(logger)

	// Create program state checker
	stateChecker := NewProgramStateChecker(client, config, logger.Named("state-checker"))

	// Create DEX instance
	dex := &DEX{
		client:        client,
		wallet:        w,
		logger:        logger.Named("pumpfun"),
		config:        config,
		errorAnalyzer: errorAnalyzer,
		stateChecker:  stateChecker,
	}

	// Fetch global account data to get fee recipient
	fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	globalAccount, err := FetchGlobalAccount(fetchCtx, client, config.Global, logger.Named("global-account"))
	if err != nil {
		logger.Warn("Failed to fetch global account data, fee recipient will not be available",
			zap.Error(err))
	} else {
		// Update fee recipient from global account data
		config.UpdateFeeRecipient(globalAccount.FeeRecipient, logger)
	}

	return dex, nil
}

// VerifyAccounts checks if all necessary accounts are properly initialized and
// exists on-chain. It validates the program state and ensures that critical accounts
// like the associated bonding curve are properly set up.
// Returns an error if any verification step fails.
func (d *DEX) VerifyAccounts(ctx context.Context) error {
	d.logger.Debug("Verifying critical accounts")

	// Check global account state
	state, err := d.stateChecker.CheckProgramState(ctx)
	if err != nil {
		return fmt.Errorf("account verification failed: %w", err)
	}

	if !state.IsReady() {
		d.logger.Warn("PumpFun program state is not ready",
			zap.String("error", state.Error),
			zap.Bool("global_initialized", state.GlobalInitialized))
		return fmt.Errorf("program state not ready: %s", state.Error)
	}

	// Verify associated bonding curve
	if d.config.AssociatedBondingCurve.IsZero() {
		d.logger.Warn("Associated bonding curve is not set, will attempt to derive it")

		// Make sure bonding curve address is valid
		if d.config.BondingCurve.IsZero() {
			return fmt.Errorf("cannot derive associated bonding curve: bonding curve address is zero")
		}

		// Derive associated bonding curve address
		derivedAddress, bump, err := deriveAssociatedCurveAddress(d.config.BondingCurve, d.config.ContractAddress)
		if err != nil {
			return fmt.Errorf("failed to derive associated bonding curve: %w", err)
		}

		d.config.AssociatedBondingCurve = derivedAddress
		d.logger.Info("Derived associated bonding curve",
			zap.String("address", derivedAddress.String()),
			zap.Uint8("bump", bump))

		// Verify the derived account exists on-chain
		accountInfo, err := d.client.GetAccountInfo(ctx, derivedAddress)
		if err != nil {
			return fmt.Errorf("failed to verify derived associated bonding curve: %w", err)
		}

		// Правильная проверка существования данных аккаунта
		if accountInfo == nil || accountInfo.Value == nil || len(accountInfo.Value.Data.GetBinary()) == 0 {
			d.logger.Warn("Derived associated bonding curve account does not exist or is empty")
			// Depending on your requirements, you might want to return an error here
			// or continue with the knowledge that the account needs to be created
		}
	}

	d.logger.Debug("All critical accounts verified successfully")
	return nil
}

// CreateAndSendTransaction creates, signs, and sends a transaction.
func CreateAndSendTransaction(ctx context.Context, client *solbc.Client, w *wallet.Wallet, instructions []solana.Instruction, logger *zap.Logger) (solana.Signature, error) {
	blockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(w.PublicKey))
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	if err := w.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	sig, err := client.SendTransaction(ctx, tx)
	if err != nil {
		// Detailed error analysis
		errorAnalyzer := solbc.NewErrorAnalyzer(logger)
		analysis := errorAnalyzer.AnalyzeRPCError(err)

		// Enhanced error detection for account initialization issues
		if isAccountNotInitializedError(err, analysis) {
			return solana.Signature{}, fmt.Errorf("account not initialized: %w", err)
		}

		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	return sig, nil
}

// isAccountNotInitializedError checks if the error is related to an uninitialized account
func isAccountNotInitializedError(err error, analysis map[string]interface{}) bool {
	if err == nil {
		return false
	}

	// Check for custom error code 3012 (AccountNotInitialized)
	if errData, ok := analysis["instruction_error"].(map[string]interface{}); ok {
		if customErr, ok := errData["Custom"].(float64); ok && customErr == 3012 {
			return true
		}
	}

	// Check logs for AccountNotInitialized messages
	if logs, ok := analysis["logs"].([]string); ok {
		for _, log := range logs {
			if strings.Contains(log, "AccountNotInitialized") ||
				strings.Contains(log, "account to be already initialized") {
				return true
			}
		}
	}

	// Check error message directly
	errStr := err.Error()
	return strings.Contains(errStr, "AccountNotInitialized") ||
		strings.Contains(errStr, "0xbc4") || // AccountNotInitialized error hex
		strings.Contains(errStr, "3012") // AccountNotInitialized error code
}

// ExecuteSnipe executes a buy operation on the Pump.fun protocol
func (d *DEX) ExecuteSnipe(ctx context.Context, amount, maxSolCost uint64) error {
	// Create a context with timeout for the entire operation
	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	d.logger.Info("Starting Pump.fun buy operation",
		zap.Uint64("amount", amount),
		zap.Uint64("max_sol_cost", maxSolCost))

	// Verify accounts before execution
	if err := d.VerifyAccounts(opCtx); err != nil {
		return fmt.Errorf("account verification failed before execution: %w", err)
	}

	// Check if fee recipient is set, if not, try to fetch it from global account
	if d.config.FeeRecipient.IsZero() {
		fetchCtx, fetchCancel := context.WithTimeout(opCtx, 10*time.Second)
		defer fetchCancel()

		d.logger.Debug("Fetching global account to get fee recipient")
		globalAccount, err := FetchGlobalAccount(fetchCtx, d.client, d.config.Global, d.logger)
		if err != nil {
			return fmt.Errorf("fee recipient not set and failed to fetch global account: %w", err)
		}

		// Update fee recipient from global account data
		d.config.UpdateFeeRecipient(globalAccount.FeeRecipient, d.logger)

		// Check if we now have a valid fee recipient
		if d.config.FeeRecipient.IsZero() {
			return fmt.Errorf("fee recipient is not available, cannot execute snipe")
		}
	}

	// Setup accounts for buy instruction
	buyAccounts := InstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
	}

	// Validate key accounts before proceeding
	if buyAccounts.Global.IsZero() {
		return fmt.Errorf("global account address is zero")
	}
	if buyAccounts.FeeRecipient.IsZero() {
		return fmt.Errorf("fee recipient address is zero")
	}
	if buyAccounts.EventAuthority.IsZero() {
		return fmt.Errorf("event authority address is zero")
	}
	if buyAccounts.AssociatedBondingCurve.IsZero() {
		return fmt.Errorf("associated bonding curve address is zero")
	}

	// ACCOUNT INITIALIZATION PHASE - in correct sequence
	d.logger.Info("Initializing required accounts")

	// Step 1: First create user ATA to receive tokens
	d.logger.Debug("Step 1: Ensuring user associated token account")
	if err := ensureUserATA(opCtx, d.client, d.wallet, d.config.Mint, d.logger); err != nil {
		return fmt.Errorf("failed to ensure user token account: %w", err)
	}

	// Step 2: Then create bonding curve ATA
	d.logger.Debug("Step 2: Ensuring bonding curve associated token account")
	if err := ensureBondingCurveATA(opCtx, d.client, d.wallet, d.config.Mint, d.config.BondingCurve, d.logger); err != nil {
		return fmt.Errorf("failed to ensure bonding curve token account: %w", err)
	}

	// Step 3: Initialize associated bonding curve with program
	d.logger.Debug("Step 3: Initializing associated bonding curve account")
	if err := d.ensureAssociatedBondingCurve(opCtx); err != nil {
		return fmt.Errorf("failed to initialize associated bonding curve: %w", err)
	}

	// After account initialization, double-check that all accounts exist
	d.logger.Debug("Verifying all accounts after initialization")
	checkCtx, checkCancel := context.WithTimeout(opCtx, 5*time.Second)
	defer checkCancel()
	
	// Verify the associated bonding curve is properly initialized
	accInfo, err := d.client.GetAccountInfo(checkCtx, d.config.AssociatedBondingCurve)
	if err != nil || accInfo.Value == nil || !accInfo.Value.Owner.Equals(d.config.ContractAddress) {
		d.logger.Warn("Associated bonding curve is still not properly initialized",
			zap.Error(err),
			zap.String("address", d.config.AssociatedBondingCurve.String()))
		
		if accInfo != nil && accInfo.Value != nil {
			d.logger.Debug("Current owner",
				zap.String("owner", accInfo.Value.Owner.String()),
				zap.String("expected", d.config.ContractAddress.String()))
		}
		
		// Try to reinitialize one more time with a slight delay
		time.Sleep(2 * time.Second)
		if initErr := d.ensureAssociatedBondingCurve(opCtx); initErr != nil {
			d.logger.Warn("Final attempt to initialize associated bonding curve failed", 
				zap.Error(initErr))
		}
	}

	// TRANSACTION EXECUTION PHASE
	d.logger.Info("Executing Pump.fun buy transaction",
		zap.Uint64("amount", amount),
		zap.Uint64("max_sol_cost", maxSolCost))

	// Build buy instruction
	buyIx, err := BuildBuyTokenInstruction(buyAccounts, d.wallet, amount, maxSolCost)
	if err != nil {
		return fmt.Errorf("failed to build buy instruction: %w", err)
	}

	// Send transaction
	txSig, err := CreateAndSendTransaction(opCtx, d.client, d.wallet, []solana.Instruction{buyIx}, d.logger)
	if err != nil {
		// Analyze the error
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)
		d.logger.Warn("Error sending buy transaction", 
			zap.Error(err),
			zap.Any("analysis", analysis))

		// If it's an uninitialized account error
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Info("AccountNotInitialized error detected, trying to recover")
			
			// Try one more time to initialize the account
			reinitCtx, reinitCancel := context.WithTimeout(opCtx, 10*time.Second)
			defer reinitCancel()
			
			if err := d.ensureAssociatedBondingCurve(reinitCtx); err != nil {
				d.logger.Error("Failed to reinitialize associated bonding curve", zap.Error(err))
				return fmt.Errorf("account initialization failed after error: %w", err)
			}
			
			// Wait a short time for blockchain state to update
			time.Sleep(2 * time.Second)
			
			// Try to execute the transaction again
			d.logger.Info("Retrying buy transaction after account initialization")
			retryTxSig, retryErr := CreateAndSendTransaction(opCtx, d.client, d.wallet, []solana.Instruction{buyIx}, d.logger)
			if retryErr != nil {
				return fmt.Errorf("failed to send transaction after reinitialization: %w", retryErr)
			}
			
			d.logger.Info("Pump.fun buy operation successful on retry",
				zap.String("tx", retryTxSig.String()),
				zap.Uint64("amount", amount))
				
			return nil
		}

		return fmt.Errorf("failed to send Pump.fun buy transaction: %w", err)
	}

	d.logger.Info("Pump.fun buy operation successful",
		zap.String("tx", txSig.String()),
		zap.Uint64("amount", amount))

	return nil
}

// ExecuteSell executes a sell operation on Pump.fun protocol
func (d *DEX) ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error {
	// Create a context with timeout for the entire operation
	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	d.logger.Info("Starting Pump.fun sell operation",
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput))

	// Check sell permission
	if !d.config.AllowSellBeforeFull {
		return fmt.Errorf("selling not allowed before 100%% bonding curve")
	}

	// Verify accounts before execution
	if err := d.VerifyAccounts(opCtx); err != nil {
		return fmt.Errorf("account verification failed before execution: %w", err)
	}

	// Check if fee recipient is set, if not, try to fetch it from global account
	if d.config.FeeRecipient.IsZero() {
		fetchCtx, fetchCancel := context.WithTimeout(opCtx, 10*time.Second)
		defer fetchCancel()

		d.logger.Debug("Fetching global account to get fee recipient")
		globalAccount, err := FetchGlobalAccount(fetchCtx, d.client, d.config.Global, d.logger)
		if err != nil {
			return fmt.Errorf("fee recipient not set and failed to fetch global account: %w", err)
		}

		// Update fee recipient from global account data
		d.config.UpdateFeeRecipient(globalAccount.FeeRecipient, d.logger)

		// Check if we now have a valid fee recipient
		if d.config.FeeRecipient.IsZero() {
			return fmt.Errorf("fee recipient is not available, cannot execute sell")
		}
	}

	// Setup accounts for sell instruction
	sellAccounts := InstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
	}

	// Validate key accounts
	if sellAccounts.Global.IsZero() {
		return fmt.Errorf("global account address is zero")
	}
	if sellAccounts.FeeRecipient.IsZero() {
		return fmt.Errorf("fee recipient address is zero")
	}
	if sellAccounts.EventAuthority.IsZero() {
		return fmt.Errorf("event authority address is zero")
	}
	if sellAccounts.AssociatedBondingCurve.IsZero() {
		return fmt.Errorf("associated bonding curve address is zero")
	}

	// ACCOUNT INITIALIZATION PHASE - in correct sequence
	d.logger.Info("Initializing required accounts")

	// Step 1: First create user ATA to receive tokens
	d.logger.Debug("Step 1: Ensuring user associated token account")
	if err := ensureUserATA(opCtx, d.client, d.wallet, d.config.Mint, d.logger); err != nil {
		return fmt.Errorf("failed to ensure user token account: %w", err)
	}

	// Step 2: Then create bonding curve ATA
	d.logger.Debug("Step 2: Ensuring bonding curve associated token account")
	if err := ensureBondingCurveATA(opCtx, d.client, d.wallet, d.config.Mint, d.config.BondingCurve, d.logger); err != nil {
		return fmt.Errorf("failed to ensure bonding curve token account: %w", err)
	}

	// Step 3: Initialize associated bonding curve with program
	d.logger.Debug("Step 3: Initializing associated bonding curve account")
	if err := d.ensureAssociatedBondingCurve(opCtx); err != nil {
		return fmt.Errorf("failed to initialize associated bonding curve: %w", err)
	}

	// TRANSACTION EXECUTION PHASE
	d.logger.Info("Executing Pump.fun sell transaction",
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput))

	// Build sell instruction
	sellIx, err := BuildSellTokenInstruction(sellAccounts, d.wallet, amount, minSolOutput)
	if err != nil {
		return fmt.Errorf("failed to build sell token instruction: %w", err)
	}

	// Send transaction
	txSig, err := CreateAndSendTransaction(opCtx, d.client, d.wallet, []solana.Instruction{sellIx}, d.logger)
	if err != nil {
		// Analyze the error
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)
		d.logger.Warn("Error sending sell transaction", 
			zap.Error(err),
			zap.Any("analysis", analysis))

		// If it's an uninitialized account error
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Info("AccountNotInitialized error detected, trying to recover")
			
			// Try one more time to initialize the account
			reinitCtx, reinitCancel := context.WithTimeout(opCtx, 10*time.Second)
			defer reinitCancel()
			
			if err := d.ensureAssociatedBondingCurve(reinitCtx); err != nil {
				d.logger.Error("Failed to reinitialize associated bonding curve", zap.Error(err))
				return fmt.Errorf("account initialization failed after error: %w", err)
			}
			
			// Wait a short time for blockchain state to update
			time.Sleep(2 * time.Second)
			
			// Try to execute the transaction again
			d.logger.Info("Retrying sell transaction after account initialization")
			retryTxSig, retryErr := CreateAndSendTransaction(opCtx, d.client, d.wallet, []solana.Instruction{sellIx}, d.logger)
			if retryErr != nil {
				return fmt.Errorf("failed to send transaction after reinitialization: %w", retryErr)
			}
			
			d.logger.Info("Pump.fun sell operation successful on retry",
				zap.String("tx", retryTxSig.String()),
				zap.Uint64("amount", amount),
				zap.Uint64("min_sol_output", minSolOutput))
				
			return nil
		}

		return fmt.Errorf("failed to send Pump.fun sell transaction: %w", err)
	}

	d.logger.Info("Pump.fun sell operation successful",
		zap.String("tx", txSig.String()),
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput))

	return nil
}

// ensureBondingCurveATA ensures the bonding curve has an associated token account
func ensureBondingCurveATA(
	ctx context.Context,
	client *solbc.Client,
	wallet *wallet.Wallet,
	mint solana.PublicKey,
	bondingCurve solana.PublicKey,
	logger *zap.Logger,
) error {
	// Get bonding curve ATA address
	bondingCurveATA, _, err := solana.FindAssociatedTokenAddress(bondingCurve, mint)
	if err != nil {
		return fmt.Errorf("failed to get bonding curve ATA address: %w", err)
	}

	// Check if account exists
	exists, err := accountExists(ctx, client, bondingCurveATA)
	if err != nil {
		return fmt.Errorf("failed to check bonding curve ATA existence: %w", err)
	}

	// If account already exists, nothing to do
	if exists {
		return nil
	}

	// Create bonding curve ATA
	logger.Info("Creating bonding curve ATA",
		zap.String("address", bondingCurveATA.String()),
		zap.String("mint", mint.String()),
		zap.String("owner", bondingCurve.String()))

	createTx, err := createAssociatedTokenAccount(
		ctx,
		client,
		wallet,
		mint,
		bondingCurve,
		logger,
	)

	if err != nil {
		return fmt.Errorf("failed to prepare bonding curve ATA creation: %w", err)
	}

	if createTx != nil {
		// Сначала подписываем транзакцию
		if err := wallet.SignTransaction(createTx); err != nil {
			return fmt.Errorf("failed to sign ATA creation transaction: %w", err)
		}

		// Send transaction и сохраняем подпись
		sig, err := client.SendTransaction(ctx, createTx)
		if err != nil {
			return fmt.Errorf("failed to create bonding curve ATA: %w", err)
		}

		// Создаем timeout для ожидания подтверждения
		waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		// Активно ждем подтверждения транзакции
		if err := client.WaitForTransactionConfirmation(waitCtx, sig, rpc.CommitmentConfirmed); err != nil {
			return fmt.Errorf("bonding curve ATA creation confirmation failed: %w", err)
		}

		logger.Info("Bonding curve ATA created successfully",
			zap.String("address", bondingCurveATA.String()),
			zap.String("signature", sig.String()))
	}

	return nil
}

// ensureUserATA ensures the user has an associated token account
func ensureUserATA(
	ctx context.Context,
	client *solbc.Client,
	wallet *wallet.Wallet,
	mint solana.PublicKey,
	logger *zap.Logger,
) error {
	// Get user ATA address
	userATA, err := wallet.GetATA(mint)
	if err != nil {
		return fmt.Errorf("failed to get user ATA address: %w", err)
	}

	// Check if account exists
	exists, err := accountExists(ctx, client, userATA)
	if err != nil {
		return fmt.Errorf("failed to check user ATA existence: %w", err)
	}

	// If account already exists, nothing to do
	if exists {
		return nil
	}

	// Create user ATA
	logger.Info("Creating user ATA", zap.String("address", userATA.String()))
	createTx, err := createAssociatedTokenAccount(
		ctx,
		client,
		wallet,
		mint,
		wallet.PublicKey,
		logger,
	)

	if err != nil {
		return fmt.Errorf("failed to prepare user ATA creation: %w", err)
	}

	if createTx != nil {
		// Сначала подписываем транзакцию
		if err := wallet.SignTransaction(createTx); err != nil {
			return fmt.Errorf("failed to sign ATA creation transaction: %w", err)
		}

		// Отправляем транзакцию и сохраняем подпись
		sig, err := client.SendTransaction(ctx, createTx)
		if err != nil {
			return fmt.Errorf("failed to create ATA: %w", err)
		}

		// Создаем таймаут для ожидания подтверждения
		waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		// Ждем подтверждение с использованием полученной подписи
		if err := client.WaitForTransactionConfirmation(waitCtx, sig, rpc.CommitmentConfirmed); err != nil {
			return fmt.Errorf("ATA creation confirmation failed: %w", err)
		}

		logger.Info("ATA created successfully",
			zap.String("address", userATA.String()),
			zap.String("signature", sig.String()))
	}

	return nil
}

// ensureAssociatedBondingCurve ensures that the associated bonding curve account exists and is properly initialized
func (d *DEX) ensureAssociatedBondingCurve(ctx context.Context) error {
	d.logger.Debug("Checking associated bonding curve account",
		zap.String("address", d.config.AssociatedBondingCurve.String()))

	// Check if account exists
	accInfo, err := d.client.GetAccountInfo(ctx, d.config.AssociatedBondingCurve)

	// If account exists and is initialized by the program, we're done
	if err == nil && accInfo.Value != nil && accInfo.Value.Owner.Equals(d.config.ContractAddress) {
		d.logger.Debug("Associated bonding curve account already initialized by program",
			zap.String("owner", accInfo.Value.Owner.String()))
		return nil
	}

	// If there was an error or the account doesn't exist, log it
	if err != nil {
		d.logger.Debug("Associated bonding curve account check failed",
			zap.Error(err))
	} else if accInfo.Value == nil || accInfo.Value.Owner.IsZero() {
		d.logger.Debug("Associated bonding curve account not initialized")
	} else {
		d.logger.Debug("Associated bonding curve account has wrong owner",
			zap.String("current_owner", accInfo.Value.Owner.String()),
			zap.String("expected_owner", d.config.ContractAddress.String()))
	}

	d.logger.Info("Creating and initializing associated bonding curve account",
		zap.String("address", d.config.AssociatedBondingCurve.String()))

	// Step 1: Get recent blockhash
	blockhash, err := d.client.GetRecentBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Step 2: Build the initialize instruction
	// This will explicitly initialize the associated bonding curve with the correct program
	initInstructions := []solana.Instruction{
		// First ensure the bonding curve ATA exists (already called)
		// Then initialize the associated bonding curve
		BuildInitializeAssociatedBondingCurveInstruction(
			d.config.Mint,
			d.config.BondingCurve,
			d.config.AssociatedBondingCurve,
			d.config.ContractAddress,
			d.wallet.PublicKey,
		),
	}

	// Create transaction
	tx, err := solana.NewTransaction(
		initInstructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create associated bonding curve init transaction: %w", err)
	}

	// Sign the transaction
	err = d.wallet.SignTransaction(tx)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction and capture the signature
	sig, err := d.client.SendTransaction(ctx, tx)
	if err != nil {
		// If there's an error, it could be because the account was created in parallel or by another user
		// We should check if it's a specific type of error and handle it accordingly
		d.logger.Warn("Error sending associated bonding curve initialization transaction",
			zap.Error(err))
		
		// Analyze the error
		if strings.Contains(err.Error(), "already in use") {
			d.logger.Info("Account already in use, this is expected if someone else initialized it")
			// Verify the account now exists and is properly owned
			verifyAccInfo, verifyErr := d.client.GetAccountInfo(ctx, d.config.AssociatedBondingCurve)
			if verifyErr == nil && verifyAccInfo.Value != nil && verifyAccInfo.Value.Owner.Equals(d.config.ContractAddress) {
				d.logger.Info("Associated bonding curve now properly initialized")
				return nil
			}
		}
		return fmt.Errorf("failed to send associated bonding curve initialization: %w", err)
	}

	// Wait for confirmation using the actual transaction signature
	d.logger.Info("Waiting for associated bonding curve initialization confirmation",
		zap.String("signature", sig.String()))
	
	confirmCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err = d.client.WaitForTransactionConfirmation(confirmCtx, sig, rpc.CommitmentConfirmed); err != nil {
		return fmt.Errorf("associated bonding curve initialization confirmation failed: %w", err)
	}

	d.logger.Info("Associated bonding curve initialized successfully",
		zap.String("address", d.config.AssociatedBondingCurve.String()))

	return nil
}
