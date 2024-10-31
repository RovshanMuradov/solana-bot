// internal/dex/raydium/instruction.go
package raydium

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	token "github.com/gagliardetto/solana-go/programs/token"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// InstructionType определяет тип инструкции Raydium
type InstructionType uint8

const (
	// Типы инструкций
	InstructionTypeSwap       InstructionType = 1
	InstructionTypeInitialize InstructionType = 2
	InstructionTypeDeposit    InstructionType = 3
	InstructionTypeWithdraw   InstructionType = 4

	// Размеры данных
	SwapInstructionSize = 17 // 1 (тип) + 8 (amountIn) + 8 (minAmountOut)
)

// SwapInstructionBuilder строит инструкции свапа
type SwapInstructionBuilder struct {
	client blockchain.Client
	logger *zap.Logger
	pool   *RaydiumPool
}

// NewSwapInstructionBuilder создает новый builder для инструкций
func NewSwapInstructionBuilder(
	client blockchain.Client,
	logger *zap.Logger,
	pool *RaydiumPool,
) *SwapInstructionBuilder {
	return &SwapInstructionBuilder{
		client: client,
		logger: logger.Named("swap-instruction-builder"),
		pool:   pool,
	}
}

// SwapInstructionAccounts содержит все необходимые аккаунты для свапа
type SwapInstructionAccounts struct {
	// Пользовательские аккаунты
	UserAuthority   solana.PublicKey
	UserSourceToken solana.PublicKey
	UserDestToken   solana.PublicKey

	// Аккаунты пула
	AmmId           solana.PublicKey
	AmmAuthority    solana.PublicKey
	AmmOpenOrders   solana.PublicKey
	AmmTargetOrders solana.PublicKey
	PoolSourceVault solana.PublicKey
	PoolDestVault   solana.PublicKey

	// Serum аккаунты
	SerumProgram     solana.PublicKey
	SerumMarket      solana.PublicKey
	SerumBids        solana.PublicKey
	SerumAsks        solana.PublicKey
	SerumEventQueue  solana.PublicKey
	SerumBaseVault   solana.PublicKey
	SerumQuoteVault  solana.PublicKey
	SerumVaultSigner solana.PublicKey
}

// SwapInstructionData содержит данные для инструкции свапа
type SwapInstructionData struct {
	Instruction  InstructionType
	AmountIn     uint64
	MinAmountOut uint64
}

// BuildSwapInstruction создает инструкцию свапа
func (b *SwapInstructionBuilder) BuildSwapInstruction(
	ctx context.Context,
	params SwapParams,
	accounts SwapInstructionAccounts,
) (solana.Instruction, error) {
	logger := b.logger.With(
		zap.String("user", accounts.UserAuthority.String()),
		zap.Uint64("amount_in", params.AmountIn),
		zap.Uint64("min_amount_out", params.MinAmountOut),
	)
	logger.Debug("Building swap instruction")

	// Валидация аккаунтов
	if err := b.validateAccounts(ctx, accounts); err != nil {
		return nil, fmt.Errorf("invalid accounts: %w", err)
	}

	// Создаем список аккаунтов в правильном порядке
	accountMetas := b.buildAccountMetas(accounts)

	// Создаем данные инструкции
	data, err := b.serializeInstructionData(params)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize instruction data: %w", err)
	}

	// Создаем инструкцию
	instruction := solana.NewInstruction(
		b.pool.AmmProgramID,
		accountMetas,
		data,
	)

	logger.Debug("Swap instruction built successfully",
		zap.Int("num_accounts", len(accountMetas)),
		zap.Int("data_size", len(data)))

	return instruction, nil
}

// validateAccounts проверяет все необходимые аккаунты
func (b *SwapInstructionBuilder) validateAccounts(
	ctx context.Context,
	accounts SwapInstructionAccounts,
) error {
	// Проверяем основные аккаунты
	if accounts.UserAuthority.IsZero() {
		return fmt.Errorf("user authority is required")
	}
	if accounts.UserSourceToken.IsZero() || accounts.UserDestToken.IsZero() {
		return fmt.Errorf("user token accounts are required")
	}
	if accounts.AmmId.IsZero() {
		return fmt.Errorf("AMM ID is required")
	}

	// Проверяем существование и владельца токен-аккаунтов
	for _, check := range []struct {
		account solana.PublicKey
		name    string
	}{
		{accounts.UserSourceToken, "source token"},
		{accounts.UserDestToken, "destination token"},
		{accounts.PoolSourceVault, "pool source vault"},
		{accounts.PoolDestVault, "pool destination vault"},
	} {
		info, err := b.client.GetAccountInfo(ctx, check.account)
		if err != nil {
			return fmt.Errorf("failed to get %s account info: %w", check.name, err)
		}
		if info.Value == nil || !info.Value.Owner.Equals(token.ProgramID) {
			return fmt.Errorf("invalid %s account", check.name)
		}
	}

	return nil
}

// buildAccountMetas создает список аккаунтов в правильном порядке
func (b *SwapInstructionBuilder) buildAccountMetas(accounts SwapInstructionAccounts) solana.AccountMetaSlice {
	return solana.AccountMetaSlice{
		// User accounts
		solana.NewAccountMeta(accounts.UserAuthority, true, true),    // Writable, Signer
		solana.NewAccountMeta(accounts.UserSourceToken, false, true), // Writable
		solana.NewAccountMeta(accounts.UserDestToken, false, true),   // Writable

		// AMM accounts
		solana.NewAccountMeta(accounts.AmmId, false, true),           // Writable
		solana.NewAccountMeta(accounts.AmmAuthority, false, false),   // Not writable
		solana.NewAccountMeta(accounts.AmmOpenOrders, false, true),   // Writable
		solana.NewAccountMeta(accounts.AmmTargetOrders, false, true), // Writable
		solana.NewAccountMeta(accounts.PoolSourceVault, false, true), // Writable
		solana.NewAccountMeta(accounts.PoolDestVault, false, true),   // Writable

		// Serum accounts
		solana.NewAccountMeta(accounts.SerumProgram, false, false),     // Not writable
		solana.NewAccountMeta(accounts.SerumMarket, false, true),       // Writable
		solana.NewAccountMeta(accounts.SerumBids, false, true),         // Writable
		solana.NewAccountMeta(accounts.SerumAsks, false, true),         // Writable
		solana.NewAccountMeta(accounts.SerumEventQueue, false, true),   // Writable
		solana.NewAccountMeta(accounts.SerumBaseVault, false, true),    // Writable
		solana.NewAccountMeta(accounts.SerumQuoteVault, false, true),   // Writable
		solana.NewAccountMeta(accounts.SerumVaultSigner, false, false), // Not writable

		// System accounts
		solana.NewAccountMeta(solana.TokenProgramID, false, false),   // Not writable
		solana.NewAccountMeta(solana.SysVarRentPubkey, false, false), // Not writable
	}
}

// serializeInstructionData сериализует данные инструкции
func (b *SwapInstructionBuilder) serializeInstructionData(params SwapParams) ([]byte, error) {
	data := make([]byte, SwapInstructionSize)

	// Записываем тип инструкции
	data[0] = byte(InstructionTypeSwap)

	// Записываем amountIn
	binary.LittleEndian.PutUint64(data[1:9], params.AmountIn)

	// Записываем minAmountOut
	binary.LittleEndian.PutUint64(data[9:17], params.MinAmountOut)

	return data, nil
}

// BuildInitializePoolInstruction создает инструкцию инициализации пула
func (b *SwapInstructionBuilder) BuildInitializePoolInstruction(
	ctx context.Context,
	nonce uint8,
	initialLPSupply uint64,
) (solana.Instruction, error) {
	// ... реализация создания инструкции инициализации пула
	return nil, nil
}

// BuildDepositInstruction создает инструкцию депозита в пул
func (b *SwapInstructionBuilder) BuildDepositInstruction(
	ctx context.Context,
	maxBaseAmount uint64,
	maxQuoteAmount uint64,
) (solana.Instruction, error) {
	// ... реализация создания инструкции депозита
	return nil, nil
}

// BuildWithdrawInstruction создает инструкцию вывода из пула
func (b *SwapInstructionBuilder) BuildWithdrawInstruction(
	ctx context.Context,
	lpAmount uint64,
) (solana.Instruction, error) {
	// ... реализация создания инструкции вывода
	return nil, nil
}

// GetRequiredAccounts собирает все необходимые аккаунты для свапа
func (b *SwapInstructionBuilder) GetRequiredAccounts(
	ctx context.Context,
	userAuthority solana.PublicKey,
	sourceMint solana.PublicKey,
	destMint solana.PublicKey,
) (*SwapInstructionAccounts, error) {
	// Находим ATA для пользователя
	sourceATA, _, err := solana.FindAssociatedTokenAddress(userAuthority, sourceMint)
	if err != nil {
		return nil, fmt.Errorf("failed to find source ATA: %w", err)
	}

	destATA, _, err := solana.FindAssociatedTokenAddress(userAuthority, destMint)
	if err != nil {
		return nil, fmt.Errorf("failed to find destination ATA: %w", err)
	}

	// Создаем структуру с аккаунтами
	accounts := &SwapInstructionAccounts{
		UserAuthority:    userAuthority,
		UserSourceToken:  sourceATA,
		UserDestToken:    destATA,
		AmmId:            b.pool.ID,
		AmmAuthority:     b.pool.Authority,
		AmmOpenOrders:    b.pool.OpenOrders,
		AmmTargetOrders:  b.pool.TargetOrders,
		PoolSourceVault:  b.pool.BaseVault,
		PoolDestVault:    b.pool.QuoteVault,
		SerumProgram:     b.pool.MarketProgramID,
		SerumMarket:      b.pool.MarketID,
		SerumBids:        b.pool.MarketBids,
		SerumAsks:        b.pool.MarketAsks,
		SerumEventQueue:  b.pool.MarketEventQueue,
		SerumBaseVault:   b.pool.MarketBaseVault,
		SerumQuoteVault:  b.pool.MarketQuoteVault,
		SerumVaultSigner: b.pool.MarketAuthority,
	}

	return accounts, nil
}

// BuildVersionedSwapInstruction создает версионированную инструкцию свапа
func (b *SwapInstructionBuilder) BuildVersionedSwapInstruction(
	ctx context.Context,
	params SwapParams,
	accounts SwapInstructionAccounts,
) (*solana.VersionedTransaction, error) {
	// ... реализация создания версионированной инструкции свапа
	return nil, nil
}

// BuildDepositInstruction создает инструкцию депозита
func (b *SwapInstructionBuilder) BuildDepositInstruction(
	ctx context.Context,
	params DepositParams,
	accounts DepositInstructionAccounts,
) (solana.Instruction, error) {
	// ... реализация создания инструкции депозита
	return nil, nil
}
