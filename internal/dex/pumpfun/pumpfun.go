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
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX is the Pump.fun DEX implementation.
type DEX struct {
	client        *solbc.Client
	wallet        *wallet.Wallet
	logger        *zap.Logger
	config        *Config
	monitor       *BondingCurveMonitor
	events        *Monitor
	graduated     bool
	raydiumClient *raydium.Client
	errorAnalyzer *solbc.ErrorAnalyzer
	stateChecker  *ProgramStateChecker // Added state checker
}

// NewDEX creates a new instance of DEX.
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, monitorInterval string) (*DEX, error) {
	interval, err := time.ParseDuration(monitorInterval)
	if err != nil {
		return nil, err
	}

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

	// Create monitor with the bonding curve address from config
	monitor := NewBondingCurveMonitor(client, logger, interval, config.BondingCurve)

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
		monitor:       monitor,
		events:        NewPumpfunMonitor(logger, interval),
		raydiumClient: nil,
		errorAnalyzer: errorAnalyzer,
		stateChecker:  stateChecker,
	}

	// NEW CODE BLOCK: Fetch global account data to get fee recipient
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

	// Verify critical accounts before returning
	if err := dex.VerifyAccounts(context.Background()); err != nil {
		logger.Warn("Account verification failed during initialization",
			zap.Error(err),
			zap.String("global_account", config.Global.String()))
		// We'll continue anyway but with a warning
	}

	return dex, nil
}

// VerifyAccounts checks if all necessary accounts are properly initialized
// VerifyAccounts checks if all necessary accounts are properly initialized
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

		// Try to find an alternative global account if current one is not initialized
		if !state.GlobalInitialized {
			d.logger.Info("Attempting to find alternative global account")
			alternativeAccount, err := d.stateChecker.FindAlternativeGlobalAccount(ctx)
			if err == nil && alternativeAccount != "" {
				d.logger.Info("Found alternative global account",
					zap.String("current", d.config.Global.String()),
					zap.String("alternative", alternativeAccount))

				// Update the configuration with the new global account
				alternativeGlobal, err := solana.PublicKeyFromBase58(alternativeAccount)
				if err == nil {
					d.config.Global = alternativeGlobal
					d.logger.Info("Updated global account in configuration")
					return nil
				}
			}
		}

		return fmt.Errorf("program state not ready: %s", state.Error)
	}

	// NEW: Verify associated bonding curve - check if it's valid
	if d.config.AssociatedBondingCurve.IsZero() {
		d.logger.Warn("Associated bonding curve is not set, will attempt to derive it")
		d.config.AssociatedBondingCurve = deriveAssociatedCurveAddress(d.config.BondingCurve, d.config.ContractAddress)
		d.logger.Info("Derived associated bonding curve",
			zap.String("address", d.config.AssociatedBondingCurve.String()))
	}

	// NEW: Try to get info about associated bonding curve
	assocInfo, err := d.client.GetAccountInfo(ctx, d.config.AssociatedBondingCurve)
	if err != nil {
		d.logger.Warn("Could not fetch associated bonding curve info, may need to be created",
			zap.String("address", d.config.AssociatedBondingCurve.String()),
			zap.Error(err))
	} else {
		d.logger.Info("Associated bonding curve info",
			zap.String("address", d.config.AssociatedBondingCurve.String()),
			zap.String("owner", assocInfo.Value.Owner.String()),
			zap.Int("data_len", len(assocInfo.Value.Data.GetBinary())))
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

	logger.Debug("Creating transaction",
		zap.Int("num_instructions", len(instructions)),
		zap.String("blockhash", blockhash.String()))

	tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(w.PublicKey))
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	logger.Debug("Signing transaction")
	if err := w.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	logger.Debug("Sending transaction")
	sig, err := client.SendTransaction(ctx, tx)
	if err != nil {
		// Detailed error analysis
		errorAnalyzer := solbc.NewErrorAnalyzer(logger)
		analysis := errorAnalyzer.AnalyzeRPCError(err)

		logger.Error("Transaction error details",
			zap.Any("error_analysis", analysis))

		// Enhanced error detection for account initialization issues
		if isAccountNotInitializedError(err, analysis) {
			logger.Error("Account initialization error detected",
				zap.String("error_type", "AccountNotInitialized"))
			return solana.Signature{}, fmt.Errorf("account not initialized: %w", err)
		}

		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	logger.Debug("Transaction sent successfully", zap.String("signature", sig.String()))
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

// ExecuteSnipe executes a snipe operation.
func (d *DEX) ExecuteSnipe(ctx context.Context, amount, maxSolCost uint64) error {
	// Verify accounts before execution
	if err := d.VerifyAccounts(ctx); err != nil {
		return fmt.Errorf("account verification failed before execution: %w", err)
	}

	// Check if fee recipient is set, if not, try to fetch it from global account
	if d.config.FeeRecipient.IsZero() {
		d.logger.Warn("Fee recipient not set, attempting to fetch from global account")
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		globalAccount, err := FetchGlobalAccount(fetchCtx, d.client, d.config.Global, d.logger)
		if err != nil {
			d.logger.Error("Failed to fetch fee recipient from global account", zap.Error(err))
			return fmt.Errorf("fee recipient not set and failed to fetch global account: %w", err)
		}

		// Update fee recipient from global account data
		d.config.UpdateFeeRecipient(globalAccount.FeeRecipient, d.logger)

		// Check if we now have a valid fee recipient
		if d.config.FeeRecipient.IsZero() {
			return fmt.Errorf("fee recipient is not available, cannot execute snipe")
		}
	}

	if d.graduated {
		d.logger.Info("Token has graduated. Redirecting snipe to Raydium.")
		if d.raydiumClient == nil {
			return fmt.Errorf("no Raydium client set for graduated token")
		}
		snipeParams := &raydium.SnipeParams{
			TokenMint:           d.config.Mint,
			SourceMint:          solana.MustPublicKeyFromBase58("SOURCE_MINT"),
			AmmAuthority:        solana.MustPublicKeyFromBase58("AMM_AUTHORITY"),
			BaseVault:           solana.MustPublicKeyFromBase58("BASE_VAULT"),
			QuoteVault:          solana.MustPublicKeyFromBase58("QUOTE_VAULT"),
			UserPublicKey:       d.wallet.PublicKey,
			PrivateKey:          &d.wallet.PrivateKey,
			UserSourceATA:       solana.MustPublicKeyFromBase58("USER_SOURCE_ATA"),
			UserDestATA:         solana.MustPublicKeyFromBase58("USER_DEST_ATA"),
			AmountInLamports:    amount,
			MinOutLamports:      maxSolCost,
			PriorityFeeLamports: 0,
		}
		_, err := d.raydiumClient.Snipe(ctx, snipeParams)
		return err
	}

	d.logger.Info("Executing Pump.fun snipe",
		zap.Uint64("amount", amount),
		zap.Uint64("max_sol_cost", maxSolCost),
		zap.String("discriminator_version", DiscriminatorVersion),
		zap.String("fee_recipient", d.config.FeeRecipient.String()),
		zap.String("event_authority", d.config.EventAuthority.String()))

	// Setup accounts for buy instruction
	buyAccounts := BuyInstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
		Logger:                 d.logger,
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

	// После проверки аккаунтов и перед созданием ATA добавить:
	if err := d.ensureAssociatedBondingCurve(ctx); err != nil {
		d.logger.Error("Failed to ensure associated bonding curve initialization",
			zap.Error(err))
		return fmt.Errorf("failed to initialize required accounts: %w", err)
	}

	// Ensure both ATAs exist first
	if err := ensureUserATA(ctx, d.client, d.wallet, d.config.Mint, d.logger); err != nil {
		return fmt.Errorf("failed to ensure user token account: %w", err)
	}

	if err := ensureBondingCurveATA(ctx, d.client, d.wallet, d.config.Mint, d.config.BondingCurve, d.logger); err != nil {
		return fmt.Errorf("failed to ensure bonding curve token account: %w", err)
	}

	// Prepare instructions for transaction - only after ATAs are confirmed
	prepInstructions := []solana.Instruction{}

	// Build buy instruction only once
	buyIx, err := BuildBuyTokenInstruction(buyAccounts, d.wallet, amount, maxSolCost)
	if err != nil {
		return fmt.Errorf("failed to build buy instruction: %w", err)
	}

	// Add buy instruction to the prepared instructions (only once)
	prepInstructions = append(prepInstructions, buyIx)

	// Send transaction with all instructions
	d.logger.Debug("Sending buy transaction",
		zap.Uint64("amount", amount),
		zap.Uint64("max_sol_cost", maxSolCost))

	txSig, err := CreateAndSendTransaction(ctx, d.client, d.wallet, prepInstructions, d.logger)
	if err != nil {
		// Analyze the error
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)
		d.logger.Debug("Transaction error analysis", zap.Any("analysis", analysis))

		// If it's an uninitialized account error
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Warn("Account initialization error detected. Trying to verify and update accounts...")

			// Verify accounts again
			verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if verifyErr := d.VerifyAccounts(verifyCtx); verifyErr != nil {
				d.logger.Error("Failed to verify accounts after initialization error",
					zap.Error(verifyErr))
				return fmt.Errorf("account verification failed after error: %w", verifyErr)
			}

			// Try again with updated configuration
			d.logger.Info("Retrying snipe with updated account configuration")
			return d.ExecuteSnipe(ctx, amount, maxSolCost)
		}

		// Log specific error details to help with debugging
		d.logger.Error("Buy transaction failed",
			zap.Error(err),
			zap.Any("error_analysis", analysis),
			zap.String("discriminator_version", DiscriminatorVersion))

		// For other types of errors just return the error
		return fmt.Errorf("failed to send Pump.fun snipe transaction: %w", err)
	}

	d.logger.Info("Pump.fun snipe transaction sent successfully",
		zap.String("tx", txSig.String()),
		zap.Uint64("amount", amount))

	go d.monitor.Start(ctx)
	go d.events.Start(ctx)

	return nil
}

// ExecuteSell executes a sell operation on Pump.fun DEX.
func (d *DEX) ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error {
	// Verify accounts before execution
	if err := d.VerifyAccounts(ctx); err != nil {
		return fmt.Errorf("account verification failed before execution: %w", err)
	}

	// Check if fee recipient is set, if not, try to fetch it from global account
	if d.config.FeeRecipient.IsZero() {
		d.logger.Warn("Fee recipient not set, attempting to fetch from global account")
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		globalAccount, err := FetchGlobalAccount(fetchCtx, d.client, d.config.Global, d.logger)
		if err != nil {
			d.logger.Error("Failed to fetch fee recipient from global account", zap.Error(err))
			return fmt.Errorf("fee recipient not set and failed to fetch global account: %w", err)
		}

		// Update fee recipient from global account data
		d.config.UpdateFeeRecipient(globalAccount.FeeRecipient, d.logger)

		// Check if we now have a valid fee recipient
		if d.config.FeeRecipient.IsZero() {
			return fmt.Errorf("fee recipient is not available, cannot execute sell")
		}
	}

	if !d.config.AllowSellBeforeFull {
		return fmt.Errorf("selling not allowed before 100%% bonding curve")
	}

	d.logger.Info("Executing Pump.fun sell",
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput),
		zap.String("discriminator_version", DiscriminatorVersion),
		zap.String("fee_recipient", d.config.FeeRecipient.String()),
		zap.String("event_authority", d.config.EventAuthority.String()))

	// Setup accounts for sell instruction
	sellAccounts := SellInstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
		Logger:                 d.logger,
	}

	// Validate key accounts before proceeding
	if sellAccounts.Global.IsZero() {
		return fmt.Errorf("global account address is zero")
	}
	if sellAccounts.FeeRecipient.IsZero() {
		return fmt.Errorf("fee recipient address is zero")
	}
	if sellAccounts.EventAuthority.IsZero() {
		return fmt.Errorf("event authority address is zero")
	}

	// Try to build sell instruction with current discriminator version
	sellIx, err := BuildSellTokenInstruction(sellAccounts, d.wallet, amount, minSolOutput)
	if err != nil {
		return fmt.Errorf("failed to build sell token instruction: %w", err)
	}

	// Send transaction with the instruction
	d.logger.Debug("Sending sell transaction",
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput))

	txSig, err := CreateAndSendTransaction(ctx, d.client, d.wallet, []solana.Instruction{sellIx}, d.logger)

	// If we get an error, check its type
	if err != nil {
		// Analyze the error
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)
		d.logger.Debug("Transaction error analysis", zap.Any("analysis", analysis))

		// If it's an uninitialized account error
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Warn("Account initialization error detected. Trying to verify and update accounts...")

			// Verify accounts again
			verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if verifyErr := d.VerifyAccounts(verifyCtx); verifyErr != nil {
				d.logger.Error("Failed to verify accounts after initialization error",
					zap.Error(verifyErr))
				return fmt.Errorf("account verification failed after error: %w", verifyErr)
			}

			// Try again with updated configuration
			d.logger.Info("Retrying sell with updated account configuration")
			return d.ExecuteSell(ctx, amount, minSolOutput)
		}

		// Log specific error details to help with debugging
		d.logger.Error("Sell transaction failed",
			zap.Error(err),
			zap.Any("error_analysis", analysis),
			zap.String("discriminator_version", DiscriminatorVersion))

		// For other types of errors just return the error
		return fmt.Errorf("failed to send Pump.fun sell transaction: %w", err)
	}

	d.logger.Info("Pump.fun sell transaction sent successfully",
		zap.String("tx", txSig.String()),
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput))

	return nil
}

// CheckForGraduation checks if the token has reached the graduation threshold.
func (d *DEX) CheckForGraduation(ctx context.Context) (bool, error) {
	state, err := d.monitor.GetCurrentState()
	if err != nil {
		return false, err
	}
	d.logger.Debug("Bonding curve progress", zap.Float64("progress", state.Progress))
	if state.Progress >= d.config.GraduationThreshold {
		if !d.graduated {
			params := &GraduateParams{
				TokenMint:           d.config.Mint,
				BondingCurveAccount: d.config.BondingCurve,
				ExtraData:           []byte{},
			}
			// Передаём кошелёк d.wallet в функцию GraduateToken
			sig, err := GraduateToken(ctx, d.client, d.wallet, d.logger, params, d.config.ContractAddress)
			if err != nil {
				d.logger.Error("Graduation transaction failed", zap.Error(err))
			} else {
				d.logger.Info("Graduation transaction sent", zap.String("signature", sig.String()))
				d.graduated = true
			}
		}
		return true, nil
	}
	return false, nil
}

// ensureBondingCurveATA ensures the bonding curve has an associated token account for the specified mint
func ensureBondingCurveATA(
	ctx context.Context,
	client *solbc.Client,
	wallet *wallet.Wallet,
	mint solana.PublicKey,
	bondingCurve solana.PublicKey,
	logger *zap.Logger,
) error {
	// Get bonding curve ATA address
	// NOTE: Using the signature that matches your codebase
	bondingCurveATA, err := getAssociatedTokenAddress(mint, bondingCurve)
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
		logger.Debug("Bonding curve ATA already exists",
			zap.String("address", bondingCurveATA.String()),
			zap.String("mint", mint.String()),
			zap.String("owner", bondingCurve.String()))
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
		// Send transaction
		sig, err := client.SendTransaction(ctx, createTx)
		if err != nil {
			return fmt.Errorf("failed to create bonding curve ATA: %w", err)
		}

		logger.Info("Bonding curve ATA created successfully",
			zap.String("signature", sig.String()))

		// Wait briefly for confirmation
		time.Sleep(2 * time.Second)
	}

	return nil
}

// ensureUserATA ensures the user has an associated token account for the specified mint
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
		logger.Debug("User ATA already exists", zap.String("address", userATA.String()))
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
		// Send transaction
		sig, err := client.SendTransaction(ctx, createTx)
		if err != nil {
			return fmt.Errorf("failed to create user ATA: %w", err)
		}

		logger.Info("User ATA created successfully", zap.String("signature", sig.String()))
		// Wait briefly for confirmation
		time.Sleep(2 * time.Second)
	}

	return nil
}

// ensureAssociatedBondingCurve ensures that the associated bonding curve account
// exists and is properly initialized before executing operations
func (d *DEX) ensureAssociatedBondingCurve(ctx context.Context) error {
	d.logger.Info("Ensuring associated bonding curve is initialized",
		zap.String("address", d.config.AssociatedBondingCurve.String()))

	// Step 1: Check if account exists
	accInfo, err := d.client.GetAccountInfo(ctx, d.config.AssociatedBondingCurve)
	if err == nil && accInfo.Value != nil && !accInfo.Value.Owner.IsZero() {
		d.logger.Info("Associated bonding curve already exists and is initialized",
			zap.String("owner", accInfo.Value.Owner.String()))
		return nil
	}

	// Step 2: Create initialization instruction with correct discriminator
	d.logger.Info("Creating initialization instruction for associated bonding curve")

	// The discriminator for "create" instruction from PumpFun SDK
	// Using values from the SDK (./src/IDL/pump-fun.json)
	discriminator := []byte{24, 30, 200, 40, 5, 28, 7, 119}

	// Create an empty data buffer with the discriminator
	instructionData := discriminator

	// Get needed accounts from config with correct writable/signer flags
	// These must match the accounts expected by the create instruction
	initializeAccounts := []*solana.AccountMeta{
		// This is a simplified list - adjust based on your contract's requirements
		{PublicKey: d.config.Global, IsSigner: false, IsWritable: false},
		{PublicKey: d.config.FeeRecipient, IsSigner: false, IsWritable: false},
		{PublicKey: d.config.Mint, IsSigner: false, IsWritable: false},
		{PublicKey: d.config.BondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: d.config.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: d.wallet.PublicKey, IsSigner: true, IsWritable: true},
		{PublicKey: d.config.EventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: AssociatedTokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: d.config.ContractAddress, IsSigner: false, IsWritable: false},
	}

	// Create instruction with proper discriminator
	initIx := solana.NewInstruction(
		d.config.ContractAddress,
		initializeAccounts,
		instructionData,
	)

	// Add debug logging for the instruction
	d.logger.Debug("Sending instruction to initialize associated bonding curve",
		zap.Binary("instruction_data", instructionData),
		zap.Int("num_accounts", len(initializeAccounts)))

	// Create and send transaction
	sig, err := CreateAndSendTransaction(ctx, d.client, d.wallet, []solana.Instruction{initIx}, d.logger)
	if err != nil {
		d.logger.Error("Failed to send transaction",
			zap.Error(err),
			zap.Binary("instruction_data", instructionData))
		return fmt.Errorf("failed to initialize associated bonding curve: %w", err)
	}

	d.logger.Info("Associated bonding curve initialization transaction sent",
		zap.String("signature", sig.String()))

	// Wait briefly for confirmation
	time.Sleep(3 * time.Second)

	// Verify the account was created successfully
	accInfo, err = d.client.GetAccountInfo(ctx, d.config.AssociatedBondingCurve)
	if err != nil || accInfo.Value == nil {
		return fmt.Errorf("failed to verify associated bonding curve initialization: %w", err)
	}

	d.logger.Info("Associated bonding curve initialized successfully",
		zap.String("owner", accInfo.Value.Owner.String()))

	return nil
}
