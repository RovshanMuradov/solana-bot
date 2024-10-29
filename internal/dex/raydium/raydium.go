// internal/dex/raydium/raydium.go

package raydium

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/programs/token"

	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

const (
	defaultTimeout  = 10 * time.Second
	maxRetries      = 3
	retryDelay      = 500 * time.Millisecond
	ataCheckTimeout = 5 * time.Second
	txSendTimeout   = 15 * time.Second
)

func NewDEX(client blockchain.Client, logger *zap.Logger, poolInfo *Pool) *DEX {
	if err := validateDEXParams(client, logger, poolInfo); err != nil {
		logger.Error("Failed to create DEX", zap.Error(err))
		return nil
	}

	return &DEX{
		client:   client,
		logger:   logger.Named("raydium-dex"),
		poolInfo: poolInfo,
	}
}

func (r *DEX) ExecuteSwap(ctx context.Context, task *types.Task, userWallet *wallet.Wallet) error {
	opCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	r.slippage = task.SlippageConfig.Value

	logger := r.logger.With(
		zap.String("task", task.TaskName),
		zap.String("wallet", userWallet.PublicKey.String()),
		zap.String("slippage_type", string(task.SlippageConfig.Type)),
		zap.Float64("slippage_value", task.SlippageConfig.Value),
	)
	logger.Info("Starting swap execution")

	// Проверяем и получаем токен-аккаунты с таймаутом
	sourceMint, targetMint, err := parseTokenAddresses(task.SourceToken, task.TargetToken)
	if err != nil {
		return fmt.Errorf("invalid token addresses: %w", err)
	}

	ataCtx, ataCancel := context.WithTimeout(opCtx, ataCheckTimeout)
	defer ataCancel()

	sourceATA, targetATA, err := r.setupTokenAccounts(ataCtx, userWallet, sourceMint, targetMint, logger)
	if err != nil {
		return fmt.Errorf("failed to setup token accounts: %w", err)
	}

	// Подготавливаем amount с учетом decimals
	amountIn := uint64(task.AmountIn * math.Pow10(task.SourceTokenDecimals))

	logger.Debug("Prepared swap amount",
		zap.Uint64("amount_in", amountIn),
		zap.String("slippage_type", string(task.SlippageConfig.Type)),
		zap.Float64("slippage_value", task.SlippageConfig.Value),
	)

	// Создаем инструкции с таймаутом
	swapCtx, swapCancel := context.WithTimeout(opCtx, txSendTimeout)
	defer swapCancel()

	// Подготавливаем все необходимые инструкции
	instructions, err := r.PrepareSwapInstructions(
		swapCtx,
		userWallet.PublicKey,
		sourceATA,
		targetATA,
		amountIn,
		task.PriorityFee,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare swap instructions: %w", err)
	}

	// Отправляем транзакцию
	signature, err := r.sendTransactionWithRetryAndConfirmation(swapCtx, userWallet, instructions, logger)
	if err != nil {
		return fmt.Errorf("failed to send swap transaction: %w", err)
	}

	logger.Info("Swap transaction sent successfully",
		zap.String("signature", signature.String()),
		zap.Float64("priority_fee", task.PriorityFee))

	return nil
}

func (r *DEX) setupTokenAccounts(
	ctx context.Context,
	wallet *wallet.Wallet,
	sourceMint, targetMint solana.PublicKey,
	logger *zap.Logger,
) (solana.PublicKey, solana.PublicKey, error) {
	sourceATA, _, err := solana.FindAssociatedTokenAddress(wallet.PublicKey, sourceMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to find source ATA: %w", err)
	}

	targetATA, _, err := solana.FindAssociatedTokenAddress(wallet.PublicKey, targetMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to find target ATA: %w", err)
	}

	// Проверяем и создаем ATA если необходимо
	if err := r.ensureATA(ctx, wallet, sourceMint, sourceATA, "source", logger); err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}

	if err := r.ensureATA(ctx, wallet, targetMint, targetATA, "target", logger); err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}

	return sourceATA, targetATA, nil
}

func (r *DEX) ensureATA(
	ctx context.Context,
	wallet *wallet.Wallet,
	mint, ata solana.PublicKey,
	ataType string,
	logger *zap.Logger,
) error {
	logger = logger.With(
		zap.String("mint", mint.String()),
		zap.String("ata", ata.String()),
		zap.String("wallet", wallet.PublicKey.String()),
	)

	// Проверяем существование ATA с повторными попытками
	exists, err := r.checkATAExists(ctx, ata, logger)
	if err != nil {
		return fmt.Errorf("failed to check %s ATA: %w", ataType, err)
	}

	if !exists {
		logger.Debug("Creating new ATA")
		// Используем правильное создание инструкции из solana-go
		instruction, err := r.createATAInstruction(wallet, mint)
		if err != nil {
			return fmt.Errorf("failed to create %s ATA instruction: %w", ataType, err)
		}

		// Отправляем транзакцию и ждем подтверждения
		signature, err := r.sendTransactionWithRetryAndConfirmation(ctx, wallet, []solana.Instruction{instruction}, logger)
		if err != nil {
			return fmt.Errorf("failed to create %s ATA: %w", ataType, err)
		}

		logger.Info("ATA created successfully",
			zap.String("signature", signature.String()))

		// Ждем появления аккаунта
		if err := r.waitForATACreation(ctx, ata, logger); err != nil {
			return fmt.Errorf("failed to confirm %s ATA creation: %w", ataType, err)
		}
	}

	return nil
}

func (r *DEX) checkATAExists(
	ctx context.Context,
	ata solana.PublicKey,
	logger *zap.Logger,
) (bool, error) {
	for attempt := 0; attempt < maxRetries; attempt++ {
		account, err := r.client.GetAccountInfo(ctx, ata)
		if err == nil && account.Value != nil {
			// Проверяем, что владелец - TokenProgram
			return account.Value.Owner == solana.TokenProgramID, nil
		}

		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(retryDelay):
				logger.Debug("Retrying ATA check", zap.Int("attempt", attempt+1))
			}
		}
	}
	return false, nil
}

func (r *DEX) createATAInstruction(
	wallet *wallet.Wallet,
	mint solana.PublicKey,
) (solana.Instruction, error) {
	// Используем билдер из solana-go
	inst := associatedtokenaccount.NewCreateInstruction(
		wallet.PublicKey, // payer
		wallet.PublicKey, // wallet address
		mint,             // token mint
	)

	// Проводим валидацию
	if err := inst.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ATA instruction: %w", err)
	}

	return inst.Build(), nil
}

func (r *DEX) waitForATACreation(
	ctx context.Context,
	ata solana.PublicKey,
	logger *zap.Logger,
) error {
	// Увеличиваем время ожидания до 2 минут
	deadline := time.Now().Add(2 * time.Minute)
	// Начальный интервал проверки
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	retryCount := 0
	maxRetries := 60 // Максимальное количество попыток

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for ATA creation after 2 minutes")
		}

		if retryCount >= maxRetries {
			return fmt.Errorf("exceeded maximum retry attempts (%d) waiting for ATA creation", maxRetries)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			account, err := r.client.GetAccountInfo(ctx, ata)
			if err != nil {
				logger.Debug("ATA verification attempt failed",
					zap.Error(err),
					zap.Int("retry", retryCount),
					zap.Time("deadline", deadline))
				retryCount++
				continue
			}

			if account.Value != nil && account.Value.Owner == solana.TokenProgramID {
				logger.Info("ATA creation confirmed",
					zap.String("ata", ata.String()),
					zap.Int("retries", retryCount))
				return nil
			}

			logger.Debug("ATA not ready yet",
				zap.String("ata", ata.String()),
				zap.Int("retry", retryCount))
			retryCount++
		}
	}
}

// PrepareSwapInstructions объединяет все инструкции для свапа
func (r *DEX) PrepareSwapInstructions(
	ctx context.Context,
	wallet solana.PublicKey,
	sourceATA solana.PublicKey,
	targetATA solana.PublicKey,
	amountIn uint64,
	priorityFee float64,
	logger *zap.Logger,
) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// Добавляем compute budget инструкции
	computeBudgetInst := computebudget.NewSetComputeUnitPriceInstruction(
		uint64(priorityFee * 1e6),
	).Build()
	instructions = append(instructions, computeBudgetInst)

	// Создаем базовую инструкцию свапа
	swapInst, err := r.PrepareSwapInstruction(
		ctx,
		wallet,
		sourceATA,
		targetATA,
		amountIn,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare swap instruction: %w", err)
	}
	instructions = append(instructions, swapInst)

	return instructions, nil
}

// PrepareSwapInstruction подготавливает базовую инструкцию свапа
func (r *DEX) PrepareSwapInstruction(
	ctx context.Context,
	wallet solana.PublicKey,
	sourceATA solana.PublicKey,
	targetATA solana.PublicKey,
	amountIn uint64,
	logger *zap.Logger,
) (solana.Instruction, error) {
	logger = logger.With(
		zap.String("wallet", wallet.String()),
		zap.String("source_ata", sourceATA.String()),
		zap.String("target_ata", targetATA.String()),
	)
	logger.Debug("Preparing swap instruction")

	// Получаем ожидаемый выход
	sourceMint, err := r.getMintFromATA(ctx, sourceATA)
	if err != nil {
		return nil, fmt.Errorf("failed to get source mint: %w", err)
	}

	targetMint, err := r.getMintFromATA(ctx, targetATA)
	if err != nil {
		return nil, fmt.Errorf("failed to get target mint: %w", err)
	}

	expectedOut, err := r.getExpectedOutput(
		ctx,
		amountIn,
		sourceMint,
		targetMint,
		r.poolInfo,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get expected output: %w", err)
	}

	// Используем slippage из структуры DEX
	minAmountOut := calculateMinimumOut(expectedOut, r.slippage)

	return r.createSwapInstruction(
		wallet,
		sourceATA,
		targetATA,
		amountIn,
		minAmountOut,
		logger,
		r.poolInfo,
	)
}

// createSwapInstruction внутренний метод для создания инструкции свапа
func (r *DEX) createSwapInstruction(
	wallet solana.PublicKey,
	sourceATA solana.PublicKey,
	targetATA solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
	poolInfo *Pool,
) (solana.Instruction, error) {
	// Существующая логика из CreateSwapInstruction
	return r.CreateSwapInstruction(
		wallet,
		sourceATA,
		targetATA,
		amountIn,
		minAmountOut,
		logger,
		poolInfo,
	)
}

// Вспомогательный метод для получения mint address из ATA
func (r *DEX) getMintFromATA(ctx context.Context, ata solana.PublicKey) (solana.PublicKey, error) {
	account, err := r.client.GetAccountInfo(ctx, ata)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get ATA info: %w", err)
	}

	if account.Value == nil || len(account.Value.Data.GetBinary()) < 64 {
		return solana.PublicKey{}, fmt.Errorf("invalid ATA account data")
	}

	var tokenAccount token.Account
	if err := bin.NewBinDecoder(account.Value.Data.GetBinary()).Decode(&tokenAccount); err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to decode ATA data: %w", err)
	}

	return tokenAccount.Mint, nil
}

func (r *DEX) sendTransactionWithRetryAndConfirmation(
	ctx context.Context,
	wallet *wallet.Wallet,
	instructions []solana.Instruction,
	logger *zap.Logger,
) (solana.Signature, error) {
	const (
		maxRetries          = 3
		sendTimeout         = 15 * time.Second
		confirmationTimeout = 60 * time.Second
	)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return solana.Signature{}, ctx.Err()
		default:
			// Создаем контекст с таймаутом для отправки
			sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
			signature, err := r.sendTransaction(sendCtx, wallet, instructions)
			cancel()

			if err != nil {
				lastErr = err
				logger.Warn("Retrying transaction send",
					zap.Int("attempt", attempt+1),
					zap.Error(err))
				time.Sleep(time.Second * time.Duration(attempt+1))
				continue
			}

			// Создаем отдельный контекст для подтверждения
			confirmCtx, cancel := context.WithTimeout(ctx, confirmationTimeout)
			defer cancel()

			// Ждем подтверждения с периодическими проверками
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-confirmCtx.Done():
					return signature, fmt.Errorf("confirmation timeout exceeded: %v", confirmCtx.Err())
				case <-ticker.C:
					status, err := r.getTransactionStatus(ctx, signature)
					if err != nil {
						logger.Debug("Failed to get transaction status",
							zap.Error(err),
							zap.String("signature", signature.String()))
						continue
					}

					// Проверяем ошибки в транзакции
					if status.Error != "" {
						return signature, fmt.Errorf("transaction failed: %s", status.Error)
					}

					// Проверяем подтверждение
					if status.Confirmations >= 1 || status.Status == "finalized" {
						logger.Debug("Transaction confirmed",
							zap.String("signature", signature.String()),
							zap.String("status", status.Status),
							zap.Uint64("confirmations", status.Confirmations))
						return signature, nil
					}

					logger.Debug("Waiting for confirmation",
						zap.String("signature", signature.String()),
						zap.String("status", status.Status),
						zap.Uint64("confirmations", status.Confirmations))
				}
			}
		}
	}

	return solana.Signature{}, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func (r *DEX) sendTransaction(
	ctx context.Context,
	wallet *wallet.Wallet,
	instructions []solana.Instruction,
) (solana.Signature, error) {
	recent, err := r.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		instructions,
		recent,
		solana.TransactionPayer(wallet.PublicKey),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(wallet.PublicKey) {
			return &wallet.PrivateKey
		}
		return nil
	})
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	opts := blockchain.TransactionOptions{
		SkipPreflight:       true,
		PreflightCommitment: solanarpc.CommitmentProcessed,
	}

	signature, err := r.client.SendTransactionWithOpts(ctx, tx, opts)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	return signature, nil
}

func validateDEXParams(client blockchain.Client, logger *zap.Logger, poolInfo *Pool) error {
	switch {
	case client == nil:
		return fmt.Errorf("client cannot be nil")
	case logger == nil:
		return fmt.Errorf("logger cannot be nil")
	case poolInfo == nil:
		return fmt.Errorf("pool info cannot be nil")
	}
	return nil
}

func parseTokenAddresses(sourceToken, targetToken string) (solana.PublicKey, solana.PublicKey, error) {
	sourceMint, err := solana.PublicKeyFromBase58(sourceToken)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid source token: %w", err)
	}

	targetMint, err := solana.PublicKeyFromBase58(targetToken)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid target token: %w", err)
	}

	return sourceMint, targetMint, nil
}

// getExpectedOutput вычисляет ожидаемый выход для свапа
func (r *DEX) getExpectedOutput(
	ctx context.Context,
	amountIn uint64,
	sourceToken, targetToken solana.PublicKey,
	poolInfo *Pool,
	logger *zap.Logger,
) (float64, error) {
	// Создаем контекст с тайм-аутом
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	logger = logger.With(
		zap.String("source_token", sourceToken.String()),
		zap.String("target_token", targetToken.String()),
		zap.Uint64("amount_in", amountIn),
	)

	// Получаем состояние пула
	poolState, err := r.getPoolState(ctx, poolInfo)
	if err != nil {
		return 0, fmt.Errorf("failed to get pool state: %w", err)
	}

	logger.Debug("Pool state retrieved",
		zap.Uint64("token_a_reserve", poolState.TokenAReserve),
		zap.Uint64("token_b_reserve", poolState.TokenBReserve),
		zap.Float64("swap_fee", poolState.SwapFee))

	// Вычисляем ожидаемый выход с учетом всех факторов
	expectedOut := r.calculateExpectedOutput(amountIn, poolState)

	logger.Debug("Expected output calculated",
		zap.Float64("expected_out", expectedOut))

	return expectedOut, nil
}

// Определяем смещения для чтения данных пула Raydium
const (
	// Смещения в байтах для различных полей в структуре пула
	DISCRIMINATOR_SIZE = 8
	STATUS_SIZE        = 1
	NONCE_SIZE         = 1
	BASE_SIZE          = DISCRIMINATOR_SIZE + STATUS_SIZE + NONCE_SIZE // 10 байт

	// Смещения для резервов
	baseVaultOffset    = BASE_SIZE + 32 + 32 + 32 // После discriminator, status, nonce и трех pubkeys
	quoteVaultOffset   = baseVaultOffset + 32
	baseReserveOffset  = quoteVaultOffset + 32 + 8 // +8 для uint64
	quoteReserveOffset = baseReserveOffset + 8
)

// getPoolState получает текущее состояние пула
func (r *DEX) getPoolState(ctx context.Context, poolInfo *Pool) (*PoolState, error) {
	poolAccount, err := r.client.GetAccountInfo(ctx, solana.MustPublicKeyFromBase58(poolInfo.AmmID))
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}

	if poolAccount == nil || poolAccount.Value == nil {
		return nil, fmt.Errorf("pool account not found")
	}

	data := poolAccount.Value.Data.GetBinary()

	// Debug полных данных
	r.logger.Debug("Full pool data",
		zap.Binary("data", data),
		zap.Int("length", len(data)))

	if len(data) < quoteReserveOffset+8 {
		return nil, fmt.Errorf("invalid pool data length: got %d, need at least %d",
			len(data), quoteReserveOffset+8)
	}

	// Читаем резервы
	baseReserve := binary.LittleEndian.Uint64(data[baseReserveOffset : baseReserveOffset+8])
	quoteReserve := binary.LittleEndian.Uint64(data[quoteReserveOffset : quoteReserveOffset+8])

	r.logger.Debug("Pool reserves offsets",
		zap.Int("base_offset", baseReserveOffset),
		zap.Int("quote_offset", quoteReserveOffset))

	r.logger.Debug("Pool reserves raw bytes",
		zap.Binary("base_bytes", data[baseReserveOffset:baseReserveOffset+8]),
		zap.Binary("quote_bytes", data[quoteReserveOffset:quoteReserveOffset+8]))

	return &PoolState{
		TokenAReserve: baseReserve,
		TokenBReserve: quoteReserve,
		SwapFee:       0.25,
	}, nil
}

// calculateExpectedOutput вычисляет ожидаемый выход на основе состояния пула
func (r *DEX) calculateExpectedOutput(amountIn uint64, state *PoolState) float64 {
	// Конвертируем все в float64 для точных вычислений
	amountInF := float64(amountIn)
	reserveInF := float64(state.TokenAReserve)
	reserveOutF := float64(state.TokenBReserve)

	// Учитываем комиссию (0.25%)
	amountInWithFee := amountInF * (1 - state.SwapFee/100)

	// Используем формулу Raydium: dy = y * dx / (x + dx)
	// где dx - входная сумма с учетом комиссии
	// x, y - резервы токенов
	numerator := reserveOutF * amountInWithFee
	denominator := reserveInF + amountInWithFee

	expectedOut := numerator / denominator

	// Применяем дополнительный запас надежности 0.5%
	return expectedOut * 0.995
}

// GetAmountOutQuote получает котировку для свапа
func (r *DEX) GetAmountOutQuote(
	ctx context.Context,
	amountIn uint64,
	sourceToken, targetToken solana.PublicKey,
) (float64, error) {
	// Создаем временный пул для получения котировки
	poolInfo := r.poolInfo
	if poolInfo == nil {
		return 0, fmt.Errorf("pool info not configured")
	}

	// Получаем ожидаемый выход
	expectedOut, err := r.getExpectedOutput(ctx, amountIn, sourceToken, targetToken, poolInfo, r.logger)
	if err != nil {
		return 0, fmt.Errorf("failed to get expected output: %w", err)
	}

	return expectedOut, nil
}

// TransactionStatus представляет статус транзакции
type TransactionStatus struct {
	Signature     string    `json:"signature"`
	Status        string    `json:"status"`
	Confirmations uint64    `json:"confirmations"`
	Slot          uint64    `json:"slot"`
	Error         string    `json:"error,omitempty"`
	Timestamp     time.Time `json:"timestamp"` // Время проверки статуса
}

// getTransactionStatus получает полный статус транзакции
func (r *DEX) getTransactionStatus(ctx context.Context, signature solana.Signature) (*TransactionStatus, error) {
	result, err := r.client.GetSignatureStatuses(ctx, signature)
	if err != nil {
		return nil, fmt.Errorf("failed to get signature status: %w", err)
	}

	now := time.Now()
	status := &TransactionStatus{
		Signature: signature.String(),
		Status:    "pending",
		Timestamp: now,
	}

	if len(result.Value) == 0 || result.Value[0] == nil {
		return status, nil
	}

	statusInfo := result.Value[0]
	if statusInfo.Err != nil {
		status.Error = fmt.Sprintf("%v", statusInfo.Err)
		status.Status = "failed"
		return status, nil
	}

	if statusInfo.Confirmations != nil {
		status.Confirmations = *statusInfo.Confirmations
	}

	if statusInfo.Slot > 0 {
		status.Slot = statusInfo.Slot
	}

	switch statusInfo.ConfirmationStatus {
	case solanarpc.ConfirmationStatusFinalized:
		status.Status = "finalized"
	case solanarpc.ConfirmationStatusConfirmed:
		status.Status = "confirmed"
	}

	return status, nil
}

// GetSignatureStatus получает детальный статус подписи
func (r *DEX) GetSignatureStatus(ctx context.Context, signature solana.Signature) (*solanarpc.GetSignatureStatusesResult, error) {
	return r.client.GetSignatureStatuses(ctx, signature)
}

// Удалены неиспользуемые функции getConfirmations и waitForTransactionConfirmation
