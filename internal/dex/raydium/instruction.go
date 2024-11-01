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

//TODO:instruction.go:
//- Упростить BuildSwapInstruction, вынести часть логики в отдельные методы
// - Добавить поддержку batch-инструкций

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

// Добавим новые параметры для инициализации пула
type InitializePoolParams struct {
	Nonce           uint8
	InitialLPSupply uint64
	UserAuthority   solana.PublicKey
}

// Добавим параметры для депозита
type DepositParams struct {
	MaxBaseAmount  uint64
	MaxQuoteAmount uint64
	UserAuthority  solana.PublicKey
}

// BuildInitializePoolInstruction создает инструкцию инициализации пула
func (b *SwapInstructionBuilder) BuildInitializePoolInstruction(
	ctx context.Context,
	params InitializePoolParams,
) (solana.Instruction, error) {
	logger := b.logger.With(
		zap.Uint8("nonce", params.Nonce),
		zap.Uint64("initial_lp_supply", params.InitialLPSupply),
		zap.String("user_authority", params.UserAuthority.String()),
	)
	logger.Debug("Building initialize pool instruction")

	// Находим пользовательский LP токен аккаунт
	userLPAccount, _, err := solana.FindAssociatedTokenAddress(
		params.UserAuthority,
		b.pool.LPMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user LP token account: %w", err)
	}

	// Сериализуем данные инструкции
	data := make([]byte, 10) // 1 (тип) + 1 (nonce) + 8 (initialLPSupply)
	data[0] = byte(InstructionTypeInitialize)
	data[1] = params.Nonce
	binary.LittleEndian.PutUint64(data[2:], params.InitialLPSupply)

	// Создаем список необходимых аккаунтов
	accountMetas := solana.AccountMetaSlice{
		solana.NewAccountMeta(b.pool.ID, true, false),                // AMM ID
		solana.NewAccountMeta(b.pool.Authority, true, false),         // AMM Authority
		solana.NewAccountMeta(b.pool.BaseVault, true, false),         // Base Token Vault
		solana.NewAccountMeta(b.pool.QuoteVault, true, false),        // Quote Token Vault
		solana.NewAccountMeta(b.pool.LPMint, true, false),            // LP Token Mint
		solana.NewAccountMeta(userLPAccount, true, false),            // User LP Token Account
		solana.NewAccountMeta(params.UserAuthority, true, true),      // User Authority (signer)
		solana.NewAccountMeta(solana.TokenProgramID, false, false),   // Token Program
		solana.NewAccountMeta(solana.SysVarRentPubkey, false, false), // Rent Sysvar
	}

	return solana.NewInstruction(
		b.pool.AmmProgramID,
		accountMetas,
		data,
	), nil
}

// BuildDepositInstruction создает инструкцию депозита в пул
func (b *SwapInstructionBuilder) BuildDepositInstruction(
	ctx context.Context,
	params DepositParams,
) (solana.Instruction, error) {
	logger := b.logger.With(
		zap.Uint64("max_base_amount", params.MaxBaseAmount),
		zap.Uint64("max_quote_amount", params.MaxQuoteAmount),
		zap.String("user_authority", params.UserAuthority.String()),
	)
	logger.Debug("Building deposit instruction")

	// Находим пользовательские токен аккаунты
	userBaseAccount, _, err := solana.FindAssociatedTokenAddress(
		params.UserAuthority,
		b.pool.BaseMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user base token account: %w", err)
	}

	userQuoteAccount, _, err := solana.FindAssociatedTokenAddress(
		params.UserAuthority,
		b.pool.QuoteMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user quote token account: %w", err)
	}

	userLPAccount, _, err := solana.FindAssociatedTokenAddress(
		params.UserAuthority,
		b.pool.LPMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user LP token account: %w", err)
	}

	// Сериализуем данные инструкции
	data := make([]byte, 17) // 1 (тип) + 8 (maxBaseAmount) + 8 (maxQuoteAmount)
	data[0] = byte(InstructionTypeDeposit)
	binary.LittleEndian.PutUint64(data[1:9], params.MaxBaseAmount)
	binary.LittleEndian.PutUint64(data[9:17], params.MaxQuoteAmount)

	// Создаем список необходимых аккаунтов
	accountMetas := solana.AccountMetaSlice{
		solana.NewAccountMeta(b.pool.ID, true, false),              // AMM ID
		solana.NewAccountMeta(b.pool.Authority, false, false),      // AMM Authority
		solana.NewAccountMeta(b.pool.BaseVault, true, false),       // Base Token Vault
		solana.NewAccountMeta(b.pool.QuoteVault, true, false),      // Quote Token Vault
		solana.NewAccountMeta(b.pool.LPMint, true, false),          // LP Token Mint
		solana.NewAccountMeta(userBaseAccount, true, false),        // User Base Token Account
		solana.NewAccountMeta(userQuoteAccount, true, false),       // User Quote Token Account
		solana.NewAccountMeta(userLPAccount, true, false),          // User LP Token Account
		solana.NewAccountMeta(params.UserAuthority, true, true),    // User Authority (signer)
		solana.NewAccountMeta(solana.TokenProgramID, false, false), // Token Program
	}

	// Добавим проверку существования аккаунтов
	if err := b.validateDepositAccounts(ctx, userBaseAccount, userQuoteAccount, userLPAccount); err != nil {
		return nil, fmt.Errorf("invalid accounts: %w", err)
	}

	return solana.NewInstruction(
		b.pool.AmmProgramID,
		accountMetas,
		data,
	), nil
}

// validateDepositAccounts проверяет существование необходимых аккаунтов
func (b *SwapInstructionBuilder) validateDepositAccounts(
	ctx context.Context,
	userBaseAccount,
	userQuoteAccount,
	userLPAccount solana.PublicKey,
) error {
	for _, check := range []struct {
		account solana.PublicKey
		name    string
	}{
		{userBaseAccount, "user base token"},
		{userQuoteAccount, "user quote token"},
		{userLPAccount, "user LP token"},
	} {
		info, err := b.client.GetAccountInfo(ctx, check.account)
		if err != nil {
			return fmt.Errorf("failed to get %s account info: %w", check.name, err)
		}
		if info.Value == nil || !info.Value.Owner.Equals(solana.TokenProgramID) {
			return fmt.Errorf("invalid %s account", check.name)
		}
	}
	return nil
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

// Параметры для вывода из пула
type WithdrawParams struct {
	LPAmount      uint64
	UserAuthority solana.PublicKey
}

// BuildWithdrawInstruction создает инструкцию вывода из пула
func (b *SwapInstructionBuilder) BuildWithdrawInstruction(
	ctx context.Context,
	params WithdrawParams,
) (solana.Instruction, error) {
	logger := b.logger.With(
		zap.Uint64("lp_amount", params.LPAmount),
		zap.String("user_authority", params.UserAuthority.String()),
	)
	logger.Debug("Building withdraw instruction")

	// Находим пользовательские токен аккаунты
	userBaseAccount, _, err := solana.FindAssociatedTokenAddress(
		params.UserAuthority,
		b.pool.BaseMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user base token account: %w", err)
	}

	userQuoteAccount, _, err := solana.FindAssociatedTokenAddress(
		params.UserAuthority,
		b.pool.QuoteMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user quote token account: %w", err)
	}

	userLPAccount, _, err := solana.FindAssociatedTokenAddress(
		params.UserAuthority,
		b.pool.LPMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user LP token account: %w", err)
	}

	// Сериализуем данные инструкции
	data := make([]byte, 9) // 1 (тип) + 8 (lpAmount)
	data[0] = byte(InstructionTypeWithdraw)
	binary.LittleEndian.PutUint64(data[1:], params.LPAmount)

	// Создаем список необходимых аккаунтов
	accountMetas := solana.AccountMetaSlice{
		solana.NewAccountMeta(b.pool.ID, true, false),              // AMM ID
		solana.NewAccountMeta(b.pool.Authority, false, false),      // AMM Authority
		solana.NewAccountMeta(b.pool.BaseVault, true, false),       // Base Token Vault
		solana.NewAccountMeta(b.pool.QuoteVault, true, false),      // Quote Token Vault
		solana.NewAccountMeta(b.pool.LPMint, true, false),          // LP Token Mint
		solana.NewAccountMeta(userBaseAccount, true, false),        // User Base Token Account
		solana.NewAccountMeta(userQuoteAccount, true, false),       // User Quote Token Account
		solana.NewAccountMeta(userLPAccount, true, false),          // User LP Token Account
		solana.NewAccountMeta(params.UserAuthority, true, true),    // User Authority (signer)
		solana.NewAccountMeta(solana.TokenProgramID, false, false), // Token Program
	}

	// Проверяем существование аккаунтов
	if err := b.validateWithdrawAccounts(ctx, userBaseAccount, userQuoteAccount, userLPAccount); err != nil {
		return nil, fmt.Errorf("invalid accounts: %w", err)
	}

	return solana.NewInstruction(
		b.pool.AmmProgramID,
		accountMetas,
		data,
	), nil
}

// validateWithdrawAccounts проверяет существование необходимых аккаунтов
func (b *SwapInstructionBuilder) validateWithdrawAccounts(
	ctx context.Context,
	userBaseAccount,
	userQuoteAccount,
	userLPAccount solana.PublicKey,
) error {
	for _, check := range []struct {
		account solana.PublicKey
		name    string
	}{
		{userBaseAccount, "user base token"},
		{userQuoteAccount, "user quote token"},
		{userLPAccount, "user LP token"},
	} {
		info, err := b.client.GetAccountInfo(ctx, check.account)
		if err != nil {
			return fmt.Errorf("failed to get %s account info: %w", check.name, err)
		}
		if info.Value == nil || !info.Value.Owner.Equals(solana.TokenProgramID) {
			return fmt.Errorf("invalid %s account", check.name)
		}
	}
	return nil
}

// BuildVersionedSwapInstruction создает версионированную инструкцию свапа
func (b *SwapInstructionBuilder) BuildVersionedSwapInstruction(
	ctx context.Context,
	params SwapParams,
	accounts SwapInstructionAccounts,
) (*solana.Transaction, error) {
	logger := b.logger.With(
		zap.String("user", accounts.UserAuthority.String()),
		zap.Uint64("amount_in", params.AmountIn),
		zap.Uint64("min_amount_out", params.MinAmountOut),
	)
	logger.Debug("Building versioned swap instruction")

	// Создаем базовую инструкцию свапа
	instruction, err := b.BuildSwapInstruction(ctx, params, accounts)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap instruction: %w", err)
	}

	// Создаем новый транзакционный билдер
	tx := solana.NewTransactionBuilder()

	// Добавляем instruction
	tx.AddInstruction(instruction)

	// Устанавливаем fee payer
	tx.SetFeePayer(accounts.UserAuthority)

	// Если у нас есть lookup table, добавляем её через TransactionAddressTables option
	if !b.pool.LookupTableID.IsZero() {
		// Создаем мапу с адресами для lookup table
		addressTables := make(map[solana.PublicKey]solana.PublicKeySlice)

		// Получаем адреса lookup table из пула и добавляем их в мапу
		addressTables[b.pool.LookupTableID] = b.pool.LookupTableAddresses

		// Добавляем опцию с address tables
		tx.WithOpt(solana.TransactionAddressTables(addressTables))
	}

	// Строим транзакцию
	transaction, err := tx.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	logger.Debug("Versioned swap transaction built successfully")
	return transaction, nil
}

// DepositInstructionAccounts аккаунты, необходимые для депозита
type DepositInstructionAccounts struct {
	// Пользовательские аккаунты
	UserAuthority  solana.PublicKey // Аккаунт пользователя, который делает депозит
	UserBaseToken  solana.PublicKey // Аккаунт base токенов пользователя
	UserQuoteToken solana.PublicKey // Аккаунт quote токенов пользователя
	UserLPToken    solana.PublicKey // Аккаунт LP токенов пользователя

	// Аккаунты пула
	AmmId           solana.PublicKey // ID пула
	AmmAuthority    solana.PublicKey // Authority пула
	AmmOpenOrders   solana.PublicKey // OpenOrders аккаунт пула
	AmmTargetOrders solana.PublicKey // TargetOrders аккаунт пула
	LPMint          solana.PublicKey // Минт LP токенов
	PoolBaseVault   solana.PublicKey // Vault для base токенов пула
	PoolQuoteVault  solana.PublicKey // Vault для quote токенов пула
}
