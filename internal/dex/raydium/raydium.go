// internal/dex/raydium/raydium.go
package raydium

import (
	"context"
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	solanaClient "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// SwapParams содержит параметры для выполнения свапа
type SwapParams struct {
	SourceMint  solana.PublicKey
	TargetMint  solana.PublicKey
	Amount      float64
	MinAmount   float64
	Decimals    int
	PriorityFee float64
}

// initializeTokenAccounts инициализирует ATA аккаунты для токенов
func (r *DEX) initializeTokenAccounts(
	_ context.Context,
	wallet solana.PublicKey,
	sourceMint, targetMint solana.PublicKey,
	logger *zap.Logger,
) (sourceATA, targetATA solana.PublicKey, err error) {
	logger.Debug("Initializing token accounts",
		zap.String("wallet", wallet.String()),
		zap.String("source_mint", sourceMint.String()),
		zap.String("target_mint", targetMint.String()))

	// Получаем ATA для source токена
	sourceATA, _, err = solana.FindAssociatedTokenAddress(wallet, sourceMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to find source ATA: %w", err)
	}

	// Получаем ATA для target токена
	targetATA, _, err = solana.FindAssociatedTokenAddress(wallet, targetMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to find target ATA: %w", err)
	}

	logger.Debug("Token accounts initialized",
		zap.String("source_ata", sourceATA.String()),
		zap.String("target_ata", targetATA.String()))

	return sourceATA, targetATA, nil
}

func NewDEX(client *solanaClient.Client, logger *zap.Logger, poolInfo *Pool) *DEX {
	fmt.Printf("\n=== Creating new Raydium DEX ===\n")
	fmt.Printf("Client nil? %v\n", client == nil)
	fmt.Printf("Logger nil? %v\n", logger == nil)
	fmt.Printf("PoolInfo nil? %v\n", poolInfo == nil)

	if client == nil {
		fmt.Println("Client is nil")
		return nil
	}

	if logger == nil {
		fmt.Println("Logger is nil")
		return nil
	}

	if poolInfo == nil {
		fmt.Println("Pool info is nil")
		return nil
	}

	fmt.Printf("Creating DEX with pool config: %+v\n", poolInfo)

	dex := &DEX{
		client:   client,
		logger:   logger,
		poolInfo: poolInfo,
	}

	fmt.Printf("Created DEX instance: %+v\n", dex)
	return dex
}

// Добавляем метод Name() для реализации интерфейса
func (r *DEX) Name() string {
	return "Raydium"
}

func (dex *DEX) validateConfiguration() error {
	if dex.client == nil {
		return fmt.Errorf("client is nil")
	}
	if dex.logger == nil {
		return fmt.Errorf("logger is nil")
	}
	if dex.poolInfo == nil {
		return fmt.Errorf("pool info is nil")
	}

	// Используем экспортированный метод
	return dex.poolInfo.ValidateAddresses()
}

func (p *Pool) ValidateAddresses() error {
	if p == nil {
		return fmt.Errorf("pool config is nil")
	}

	// Проверяем что используется правильный AmmProgramID
	if p.AmmProgramID != "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C" {
		return fmt.Errorf("incorrect AmmProgramID: expected CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C, got %s", p.AmmProgramID)
	}

	// Проверяем что используется правильный SerumProgramID (OpenBook)
	if p.SerumProgramID != "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin" {
		return fmt.Errorf("incorrect SerumProgramID: expected 9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin, got %s", p.SerumProgramID)
	}

	addresses := map[string]string{
		"AmmProgramID":          p.AmmProgramID,
		"AmmID":                 p.AmmID,
		"AmmAuthority":          p.AmmAuthority,
		"AmmOpenOrders":         p.AmmOpenOrders,
		"AmmTargetOrders":       p.AmmTargetOrders,
		"PoolCoinTokenAccount":  p.PoolCoinTokenAccount,
		"PoolPcTokenAccount":    p.PoolPcTokenAccount,
		"SerumProgramID":        p.SerumProgramID,
		"SerumMarket":           p.SerumMarket,
		"SerumBids":             p.SerumBids,
		"SerumAsks":             p.SerumAsks,
		"SerumEventQueue":       p.SerumEventQueue,
		"SerumCoinVaultAccount": p.SerumCoinVaultAccount,
		"SerumPcVaultAccount":   p.SerumPcVaultAccount,
		"SerumVaultSigner":      p.SerumVaultSigner,
	}

	fmt.Printf("DEBUG: Validating pool addresses:\n")
	for name, addr := range addresses {
		if addr == "" {
			return fmt.Errorf("%s address is empty", name)
		}

		pubKey, err := solana.PublicKeyFromBase58(addr)
		if err != nil {
			return fmt.Errorf("invalid %s address %s: %w", name, addr, err)
		}
		fmt.Printf("DEBUG: %s: %s (valid)\n", name, pubKey.String())
	}

	return nil
}

func (r *DEX) PrepareSwapInstruction(
	ctx context.Context,
	userWallet solana.PublicKey,
	userSourceTokenAccount solana.PublicKey,
	userDestinationTokenAccount solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
) (solana.Instruction, error) {
	logger.Debug("Starting PrepareSwapInstruction",
		zap.String("user_wallet", userWallet.String()),
		zap.String("source_account", userSourceTokenAccount.String()),
		zap.String("destination_account", userDestinationTokenAccount.String()),
		zap.Uint64("amount_in", amountIn),
		zap.Uint64("min_amount_out", minAmountOut))

	// Проверяем валидность всех публичных ключей
	if userWallet.IsZero() {
		return nil, fmt.Errorf("invalid user wallet address")
	}

	if userSourceTokenAccount.IsZero() {
		return nil, fmt.Errorf("invalid source token account")
	}

	if userDestinationTokenAccount.IsZero() {
		return nil, fmt.Errorf("invalid destination token account")
	}

	// Проверяем конфигурацию DEX
	if r == nil {
		return nil, fmt.Errorf("DEX instance is nil")
	}

	if r.poolInfo == nil {
		return nil, fmt.Errorf("pool info is nil")
	}

	// Создаем канал для получения результата
	type result struct {
		instruction solana.Instruction
		err         error
	}
	resCh := make(chan result, 1)

	// Запускаем подготовку инструкции в отдельной горутине
	go func() {
		instruction, err := r.CreateSwapInstruction(
			userWallet,
			userSourceTokenAccount,
			userDestinationTokenAccount,
			amountIn,
			minAmountOut,
			logger,
			r.poolInfo,
		)
		resCh <- result{instruction, err}
	}()

	// Ожидаем либо завершения операции, либо отмены контекста
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("preparation cancelled: %w", ctx.Err())
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to create swap instruction: %w", res.err)
		}
		return res.instruction, nil
	}
}

// ExecuteSwap выполняет свап токенов
func (r *DEX) ExecuteSwap(
	ctx context.Context,
	task *types.Task,
	userWallet *wallet.Wallet,
) error {
	if err := ValidateTask(task); err != nil {
		return fmt.Errorf("invalid task parameters: %w", err)
	}

	if err := ValidatePool(r.poolInfo); err != nil {
		return fmt.Errorf("invalid pool configuration: %w", err)
	}
	logger := r.logger.With(
		zap.String("task_name", task.TaskName),
		zap.String("wallet", userWallet.PublicKey.String()))

	logger.Info("Starting swap execution")

	// Конвертируем строковые адреса токенов в PublicKey
	sourceMint, err := solana.PublicKeyFromBase58(task.SourceToken)
	if err != nil {
		return fmt.Errorf("invalid source token address: %w", err)
	}

	targetMint, err := solana.PublicKeyFromBase58(task.TargetToken)
	if err != nil {
		return fmt.Errorf("invalid target token address: %w", err)
	}

	// Инициализируем токен аккаунты
	sourceATA, targetATA, err := r.initializeTokenAccounts(
		ctx,
		userWallet.PublicKey,
		sourceMint,
		targetMint,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize token accounts: %w", err)
	}

	// Обновляем task с полученными ATA
	task.UserSourceTokenAccount = sourceATA
	task.UserDestinationTokenAccount = targetATA

	// Конвертируем float64 в uint64 с учетом десятичных знаков
	amountIn := uint64(task.AmountIn * math.Pow10(task.SourceTokenDecimals))
	minAmountOut := uint64(task.MinAmountOut * math.Pow10(task.TargetTokenDecimals))

	logger.Debug("Preparing swap parameters",
		zap.Uint64("amount_in_raw", amountIn),
		zap.Uint64("min_amount_out_raw", minAmountOut),
		zap.String("source_ata", sourceATA.String()),
		zap.String("target_ata", targetATA.String()))

	// Подготавливаем инструкцию свапа
	swapInstruction, err := r.PrepareSwapInstruction(
		ctx,
		userWallet.PublicKey,
		sourceATA,
		targetATA,
		amountIn,
		minAmountOut,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare swap instruction: %w", err)
	}

	// Отправляем транзакцию
	return r.PrepareAndSendTransaction(ctx, task, userWallet, logger, swapInstruction)
}
