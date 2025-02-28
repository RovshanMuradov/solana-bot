// ==============================================
// File: internal/dex/pumpfun/pumpfun.go
// ==============================================

package pumpfun

import (
	"context"
	"encoding/binary"
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
		zap.String("discriminator_version", DiscriminatorVersion))

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

	// Try to build buy instruction with current discriminator version
	buyIx, err := BuildBuyTokenInstruction(buyAccounts, d.wallet, amount, maxSolCost)
	if err != nil {
		return fmt.Errorf("failed to build buy instruction: %w", err)
	}

	// Send transaction with the instruction
	txSig, err := CreateAndSendTransaction(ctx, d.client, d.wallet, []solana.Instruction{buyIx}, d.logger)

	// Если мы получаем ошибку, проверяем ее тип
	if err != nil {
		// Анализируем ошибку
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)

		// Если это ошибка неинициализированного аккаунта
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Warn("Account initialization error detected. Trying to verify and update accounts...")

			// Проверяем аккаунты снова
			verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if verifyErr := d.VerifyAccounts(verifyCtx); verifyErr != nil {
				d.logger.Error("Failed to verify accounts after initialization error",
					zap.Error(verifyErr))
				return fmt.Errorf("account verification failed after error: %w", verifyErr)
			}

			// Пробуем снова с обновленной конфигурацией
			d.logger.Info("Retrying snipe with updated account configuration")
			return d.ExecuteSnipe(ctx, amount, maxSolCost)
		}

		// Для других типов ошибок просто возвращаем ошибку
		return fmt.Errorf("failed to send Pump.fun snipe transaction: %w", err)
	}

	d.logger.Info("Pump.fun snipe transaction sent", zap.String("tx", txSig.String()))

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

	if !d.config.AllowSellBeforeFull {
		return fmt.Errorf("selling not allowed before 100%% bonding curve")
	}

	d.logger.Info("Executing Pump.fun sell",
		zap.Uint64("amount", amount),
		zap.Uint64("min_sol_output", minSolOutput),
		zap.String("discriminator_version", DiscriminatorVersion))

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

	// Try to build sell instruction with current discriminator version
	sellIx, err := BuildSellTokenInstruction(sellAccounts, d.wallet, amount, minSolOutput)
	if err != nil {
		return fmt.Errorf("failed to build sell token instruction: %w", err)
	}

	// Send transaction with the instruction
	txSig, err := CreateAndSendTransaction(ctx, d.client, d.wallet, []solana.Instruction{sellIx}, d.logger)

	// Если мы получаем ошибку, проверяем ее тип
	if err != nil {
		// Анализируем ошибку
		analysis := d.errorAnalyzer.AnalyzeRPCError(err)

		// Если это ошибка неинициализированного аккаунта
		if isAccountNotInitializedError(err, analysis) {
			d.logger.Warn("Account initialization error detected. Trying to verify and update accounts...")

			// Проверяем аккаунты снова
			verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if verifyErr := d.VerifyAccounts(verifyCtx); verifyErr != nil {
				d.logger.Error("Failed to verify accounts after initialization error",
					zap.Error(verifyErr))
				return fmt.Errorf("account verification failed after error: %w", verifyErr)
			}

			// Пробуем снова с обновленной конфигурацией
			d.logger.Info("Retrying sell with updated account configuration")
			return d.ExecuteSell(ctx, amount, minSolOutput)
		}

		// Для других типов ошибок просто возвращаем ошибку
		return fmt.Errorf("failed to send Pump.fun sell transaction: %w", err)
	}

	d.logger.Info("Pump.fun sell transaction sent", zap.String("tx", txSig.String()))
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

// isFallbackNotFoundError checks if the error is a fallback not found error
func isFallbackNotFoundError(errorAnalyzer *solbc.ErrorAnalyzer, err error) bool {
	if err == nil {
		return false
	}

	// Analyze the error
	analysis := errorAnalyzer.AnalyzeRPCError(err)

	// Check for Anchor error of type InstructionFallbackNotFound
	if anchorErr, ok := analysis["anchor_error"].(solbc.AnchorError); ok {
		return anchorErr.Code == 101 && anchorErr.Name == "InstructionFallbackNotFound"
	}

	// For backward compatibility, check error message
	return strings.Contains(fmt.Sprint(err), "InstructionFallbackNotFound")
}

// tryDiscriminatorDiscovery tries various discriminator formats to find the right one
func tryDiscriminatorDiscovery(ctx context.Context, client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, amount, maxSolCost uint64) error {
	logger.Info("Starting discriminator discovery")

	// Add well-known method name candidates to try
	methodNamesToTry := []string{
		"buy", "buy_tokens", "buyTokens", "purchase", "swap", "snipe",
		"global:buy", "global:buy_tokens", "global:purchase", "global:swap",
	}

	for _, methodName := range methodNamesToTry {
		// Calculate discriminator for this method name
		discriminator := calculateMethodDiscriminator(methodName)

		logger.Debug("Trying method discriminator",
			zap.String("method", methodName),
			zap.String("discriminator", fmt.Sprintf("%x", discriminator)))

		// Get user's associated token account
		associatedUser, err := w.GetATA(config.Mint)
		if err != nil {
			logger.Error("Failed to get ATA during discovery", zap.Error(err))
			continue
		}

		// Create custom data for this method
		data := make([]byte, len(discriminator))
		copy(data, discriminator)

		// Add amount
		amountBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(amountBytes, amount)
		data = append(data, amountBytes...)

		// Add max SOL cost
		maxSolBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(maxSolBytes, maxSolCost)
		data = append(data, maxSolBytes...)

		// Create account meta list (same as in BuildBuyTokenInstruction)
		insAccounts := []*solana.AccountMeta{
			{PublicKey: config.Global, IsSigner: false, IsWritable: false},
			{PublicKey: config.FeeRecipient, IsSigner: false, IsWritable: true},
			{PublicKey: config.Mint, IsSigner: false, IsWritable: false},
			{PublicKey: config.BondingCurve, IsSigner: false, IsWritable: true},
			{PublicKey: config.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
			{PublicKey: associatedUser, IsSigner: false, IsWritable: true},
			{PublicKey: w.PublicKey, IsSigner: true, IsWritable: true},
			{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
			{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
			{PublicKey: SysvarRentPubkey, IsSigner: false, IsWritable: false},
			{PublicKey: config.EventAuthority, IsSigner: false, IsWritable: false},
			{PublicKey: config.ContractAddress, IsSigner: false, IsWritable: false},
		}

		// Create instruction
		customIx := solana.NewInstruction(config.ContractAddress, insAccounts, data)

		// Try to send transaction
		txSig, err := CreateAndSendTransaction(ctx, client, w, []solana.Instruction{customIx}, logger)
		if err == nil {
			// Success! Update the known discriminator for future use
			logger.Info("Discriminator discovery successful!",
				zap.String("method", methodName),
				zap.String("tx", txSig.String()))

			// Update the discriminator map
			BuyDiscriminators["discovered"] = discriminator
			DiscriminatorVersion = "discovered"

			// Log the result for future reference
			logger.Info("✅ SUCCESS: Add this discriminator to your code",
				zap.String("method", methodName),
				zap.String("hex", fmt.Sprintf("%x", discriminator)))

			return nil
		}

		// If not a fallback error, this might be a different error (e.g. account error)
		errorAnalyzer := solbc.NewErrorAnalyzer(logger)
		if !isFallbackNotFoundError(errorAnalyzer, err) {
			logger.Warn("Discovery attempt failed with non-fallback error",
				zap.String("method", methodName),
				zap.Error(err))
		}
	}

	logger.Error("All discriminator discovery attempts failed")
	return fmt.Errorf("discriminator discovery failed - no matching method found")
}
