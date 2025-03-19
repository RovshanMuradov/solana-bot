// ==============================================
// File: internal/dex/pumpfun/pumpfun.go
// ==============================================

package pumpfun

import (
	"context"
	"fmt"
	"regexp"
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
	}
	
	// Verify both bonding curve accounts are properly initialized
	if err := d.ensureAssociatedBondingCurve(ctx); err != nil {
		d.logger.Error("Failed to verify bonding curve accounts",
			zap.Error(err),
			zap.String("bonding_curve", d.config.BondingCurve.String()),
			zap.String("associated_bonding_curve", d.config.AssociatedBondingCurve.String()))
		
		// Add more detailed error message about required account setup
		return fmt.Errorf("bonding curve verification failed: %w. According to Pump.fun protocol, "+
			"associated bonding curve accounts must be created during token creation by the token creator. "+
			"This token cannot be interacted with using this protocol until it has a proper associated bonding curve", err)
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

// extractErrorDetails extracts detailed information from Solana error messages
func extractErrorDetails(errMsg string) map[string]string {
	details := make(map[string]string)

	// Look for AnchorError patterns
	if strings.Contains(errMsg, "AnchorError") {
		// Extract account information
		accountMatch := regexp.MustCompile(`caused by account: ([\w_-]+)`).FindStringSubmatch(errMsg)
		if len(accountMatch) > 1 {
			details["account"] = accountMatch[1]
		}

		// Extract error code
		codeMatch := regexp.MustCompile(`Error Code: ([\w]+)`).FindStringSubmatch(errMsg)
		if len(codeMatch) > 1 {
			details["error_code"] = codeMatch[1]
		}

		// Extract error number
		numberMatch := regexp.MustCompile(`Error Number: (\d+)`).FindStringSubmatch(errMsg)
		if len(numberMatch) > 1 {
			details["error_number"] = numberMatch[1]
		}

		// Extract error message
		msgMatch := regexp.MustCompile(`Error Message: (.+?)(?:\.|$)`).FindStringSubmatch(errMsg)
		if len(msgMatch) > 1 {
			details["error_message"] = msgMatch[1]
		}
	}

	return details
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

	// ACCOUNT INITIALIZATION PHASE
	d.logger.Info("Preparing required accounts")

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

	// Skip direct VerifyBondingCurveInstruction call since we're now doing full verification in VerifyAccounts
	d.logger.Debug("Step 3: Bonding curve accounts were already verified in initial verification step")
	
	// Additional diagnostic logging to help understand account states
	diagInfo := map[string]interface{}{}
	
	// Get information about bonding curve
	bcInfo, bcErr := d.client.GetAccountInfo(opCtx, d.config.BondingCurve)
	if bcErr == nil && bcInfo != nil && bcInfo.Value != nil {
		diagInfo["bonding_curve_exists"] = true
		diagInfo["bonding_curve_owner"] = bcInfo.Value.Owner.String()
		diagInfo["bonding_curve_data_size"] = len(bcInfo.Value.Data.GetBinary())
	} else {
		diagInfo["bonding_curve_exists"] = false
		if bcErr != nil {
			diagInfo["bonding_curve_error"] = bcErr.Error()
		}
	}
	
	// Get information about associated bonding curve
	abcInfo, abcErr := d.client.GetAccountInfo(opCtx, d.config.AssociatedBondingCurve)
	if abcErr == nil && abcInfo != nil && abcInfo.Value != nil {
		diagInfo["associated_bonding_curve_exists"] = true
		diagInfo["associated_bonding_curve_owner"] = abcInfo.Value.Owner.String()
		diagInfo["associated_bonding_curve_data_size"] = len(abcInfo.Value.Data.GetBinary())
	} else {
		diagInfo["associated_bonding_curve_exists"] = false
		if abcErr != nil {
			diagInfo["associated_bonding_curve_error"] = abcErr.Error()
		}
	}
	
	// Log the diagnostic information
	d.logger.Info("Account diagnostic information for transaction", zap.Any("diagnostics", diagInfo))

	// TRANSACTION EXECUTION PHASE
	d.logger.Info("Executing Pump.fun buy transaction",
		zap.Uint64("amount", amount),
		zap.Uint64("max_sol_cost", maxSolCost))

	// Set compute budget instructions as recommended by analysis
	// This is critical for proper transaction execution on Solana
	computeBudgetInstructions, err := CreateComputeBudgetInstructions(
		62000,  // Compute unit limit (slightly more than observed 61,368 units)
		100000, // Price in micro-lamports (0.1 lamports per unit)
	)
	if err != nil {
		return fmt.Errorf("failed to create compute budget instructions: %w", err)
	}

	// Build buy instruction
	buyIx, err := BuildBuyTokenInstruction(buyAccounts, d.wallet, amount, maxSolCost)
	if err != nil {
		return fmt.Errorf("failed to build buy instruction: %w", err)
	}

	// Create instruction array with compute budget settings first
	instructions := computeBudgetInstructions
	instructions = append(instructions, buyIx)

	// Log transaction details for debugging
	d.logger.Info("Sending transaction with compute budget settings",
		zap.Uint64("compute_units", 62000),
		zap.String("compute_price", "0.1 lamports/unit"))

	// Send transaction with all instructions
	txSig, err := CreateAndSendTransaction(opCtx, d.client, d.wallet, instructions, d.logger)
	if err != nil {
		// Analyze the error
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)
		d.logger.Warn("Error sending buy transaction",
			zap.Error(err),
			zap.Any("analysis", analysis))

		// If it's an uninitialized account error, this means our verification wasn't sufficient
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Error("Account initialization error detected for buy operation",
				zap.Error(err))

			// Re-check accounts to provide detailed diagnostics
			bcInfo, _ := d.client.GetAccountInfo(ctx, d.config.BondingCurve)
			abcInfo, _ := d.client.GetAccountInfo(ctx, d.config.AssociatedBondingCurve)
			userATA, _ := d.wallet.GetATA(d.config.Mint)
			ataInfo, _ := d.client.GetAccountInfo(ctx, userATA)

			// Log detailed diagnostic info
			d.logger.Info("Account diagnostic information",
				zap.Bool("bonding_curve_exists", bcInfo != nil && bcInfo.Value != nil),
				zap.Bool("associated_bonding_curve_exists", abcInfo != nil && abcInfo.Value != nil),
				zap.Bool("user_ata_exists", ataInfo != nil && ataInfo.Value != nil))

			return fmt.Errorf("transaction failed due to uninitialized account: %w", err)
		}

		// Handle other types of errors
		if strings.Contains(err.Error(), "Custom program error: 0x1771") ||
		   strings.Contains(err.Error(), "Custom program error: 0x1772") {
			return fmt.Errorf("transaction rejected due to slippage protection: %w", err)
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

	// ACCOUNT INITIALIZATION PHASE
	d.logger.Info("Preparing required accounts")

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

	// Skip direct VerifyBondingCurveInstruction call since we're now doing full verification in VerifyAccounts
	d.logger.Debug("Step 3: Bonding curve accounts were already verified in initial verification step")
	
	// Additional diagnostic logging to help understand account states
	diagInfo := map[string]interface{}{}
	
	// Get information about bonding curve
	bcInfo, bcErr := d.client.GetAccountInfo(opCtx, d.config.BondingCurve)
	if bcErr == nil && bcInfo != nil && bcInfo.Value != nil {
		diagInfo["bonding_curve_exists"] = true
		diagInfo["bonding_curve_owner"] = bcInfo.Value.Owner.String()
		diagInfo["bonding_curve_data_size"] = len(bcInfo.Value.Data.GetBinary())
	} else {
		diagInfo["bonding_curve_exists"] = false
		if bcErr != nil {
			diagInfo["bonding_curve_error"] = bcErr.Error()
		}
	}
	
	// Get information about associated bonding curve
	abcInfo, abcErr := d.client.GetAccountInfo(opCtx, d.config.AssociatedBondingCurve)
	if abcErr == nil && abcInfo != nil && abcInfo.Value != nil {
		diagInfo["associated_bonding_curve_exists"] = true
		diagInfo["associated_bonding_curve_owner"] = abcInfo.Value.Owner.String()
		diagInfo["associated_bonding_curve_data_size"] = len(abcInfo.Value.Data.GetBinary())
	} else {
		diagInfo["associated_bonding_curve_exists"] = false
		if abcErr != nil {
			diagInfo["associated_bonding_curve_error"] = abcErr.Error()
		}
	}
	
	// Log the diagnostic information
	d.logger.Info("Account diagnostic information for transaction", zap.Any("diagnostics", diagInfo))

	// TRANSACTION EXECUTION PHASE
	d.logger.Info("Executing Pump.fun sell transaction",
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput))

	// Set compute budget instructions as recommended by analysis
	// This is critical for proper transaction execution on Solana
	computeBudgetInstructions, err := CreateComputeBudgetInstructions(
		62000,  // Compute unit limit (slightly more than observed 61,368 units)
		100000, // Price in micro-lamports (0.1 lamports per unit)
	)
	if err != nil {
		return fmt.Errorf("failed to create compute budget instructions: %w", err)
	}

	// Build sell instruction
	sellIx, err := BuildSellTokenInstruction(sellAccounts, d.wallet, amount, minSolOutput)
	if err != nil {
		return fmt.Errorf("failed to build sell token instruction: %w", err)
	}

	// Create instruction array with compute budget settings first
	instructions := computeBudgetInstructions
	instructions = append(instructions, sellIx)

	// Log transaction details for debugging
	d.logger.Info("Sending transaction with compute budget settings",
		zap.Uint64("compute_units", 62000),
		zap.String("compute_price", "0.1 lamports/unit"))

	// Send transaction with all instructions
	txSig, err := CreateAndSendTransaction(opCtx, d.client, d.wallet, instructions, d.logger)
	if err != nil {
		// Analyze the error
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)
		d.logger.Warn("Error sending sell transaction",
			zap.Error(err),
			zap.Any("analysis", analysis))

		// If it's an uninitialized account error, this means our verification wasn't sufficient
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Error("Account initialization error detected",
				zap.Error(err))

			// Re-check accounts to provide detailed diagnostics
			bcInfo, _ := d.client.GetAccountInfo(ctx, d.config.BondingCurve)
			abcInfo, _ := d.client.GetAccountInfo(ctx, d.config.AssociatedBondingCurve)
			userATA, _ := d.wallet.GetATA(d.config.Mint)
			ataInfo, _ := d.client.GetAccountInfo(ctx, userATA)

			// Log detailed diagnostic info
			d.logger.Info("Account diagnostic information",
				zap.Bool("bonding_curve_exists", bcInfo != nil && bcInfo.Value != nil),
				zap.Bool("associated_bonding_curve_exists", abcInfo != nil && abcInfo.Value != nil),
				zap.Bool("user_ata_exists", ataInfo != nil && ataInfo.Value != nil))

			return fmt.Errorf("transaction failed due to uninitialized account: %w", err)
		}

		// Handle other types of errors
		if strings.Contains(err.Error(), "Custom program error: 0x1771") ||
		   strings.Contains(err.Error(), "Custom program error: 0x1772") {
			return fmt.Errorf("transaction rejected due to slippage protection: %w", err)
		}

		if strings.Contains(err.Error(), "Custom program error: 0x1774") ||
		   strings.Contains(err.Error(), "BondingCurveComplete") {
			return fmt.Errorf("bonding curve has already completed and liquidity migrated: %w", err)
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

// ensureAssociatedBondingCurve verifies that the associated bonding curve account exists and is properly initialized
// Using the new dynamic discovery pattern for more reliable operation
func (d *DEX) ensureAssociatedBondingCurve(ctx context.Context) error {
	d.logger.Debug("Verifying bonding curve accounts with dynamic discovery",
		zap.String("bonding_curve", d.config.BondingCurve.String()),
		zap.String("associated_bonding_curve", d.config.AssociatedBondingCurve.String()))

	// Use the IsTokenEligibleForPumpfun function which implements the full verification pipeline
	eligible, discoveredAddress, err := IsTokenEligibleForPumpfun(
		ctx,
		d.client,
		d.config.Mint,
		d.config.BondingCurve,
		d.config.ContractAddress,
		d.logger,
	)

	if err != nil {
		d.logger.Error("Token eligibility check failed",
			zap.String("mint", d.config.Mint.String()),
			zap.String("bonding_curve", d.config.BondingCurve.String()),
			zap.Error(err))
		return fmt.Errorf("token eligibility verification failed: %w", err)
	}

	if !eligible {
		d.logger.Error("Token is not eligible for Pump.fun operations",
			zap.String("mint", d.config.Mint.String()),
			zap.String("bonding_curve", d.config.BondingCurve.String()))
		return fmt.Errorf("token is not eligible for Pump.fun operations; required accounts are not properly initialized")
	}

	// Update the associated bonding curve address if it was dynamically discovered
	if d.config.AssociatedBondingCurve.IsZero() || !d.config.AssociatedBondingCurve.Equals(discoveredAddress) {
		d.logger.Info("Updating associated bonding curve address from discovery",
			zap.String("old_address", d.config.AssociatedBondingCurve.String()),
			zap.String("discovered_address", discoveredAddress.String()))
		d.config.AssociatedBondingCurve = discoveredAddress
	}

	d.logger.Info("Bonding curve accounts successfully verified",
		zap.String("bonding_curve", d.config.BondingCurve.String()),
		zap.String("associated_bonding_curve", d.config.AssociatedBondingCurve.String()))

	return nil
}
