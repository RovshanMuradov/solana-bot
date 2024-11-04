// internal/dex/raydium/client.go - это пакет, который содержит в себе реализацию клиента для работы с декстером Raydium
package raydium

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// NewRaydiumClient создает новый экземпляр клиента Raydium
func NewRaydiumClient(rpcEndpoint string, wallet solana.PrivateKey, logger *zap.Logger) (*RaydiumClient, error) {
	logger = logger.Named("raydium-client")

	// Создаем базового клиента через фабрику
	solClient, err := solbc.NewClient([]string{rpcEndpoint}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana client: %w", err)
	}

	opts := &clientOptions{
		timeout:     30 * time.Second,
		retries:     3,
		priorityFee: 1000,
		commitment:  solanarpc.CommitmentConfirmed,
	}

	return &RaydiumClient{
		client:  solClient,
		logger:  logger,
		options: opts,
	}, nil
}

// GetPool получает информацию о пуле по базовому и котируемому токенам
func (c *RaydiumClient) GetPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*RaydiumPool, error) {
	c.logger.Debug("getting raydium pool info",
		zap.String("baseMint", baseMint.String()),
		zap.String("quoteMint", quoteMint.String()),
	)

	// Получаем программные аккаунты через интерфейс
	accounts, err := c.client.GetProgramAccounts(
		ctx,
		solana.MustPublicKeyFromBase58(RAYDIUM_V4_PROGRAM_ID),
		solanarpc.GetProgramAccountsOpts{
			Filters: []solanarpc.RPCFilter{
				{
					DataSize: 388, // Размер данных аккаунта пула
				},
				{
					Memcmp: &solanarpc.RPCFilterMemcmp{
						Offset: 8,
						Bytes:  baseMint.Bytes(),
					},
				},
				{
					Memcmp: &solanarpc.RPCFilterMemcmp{
						Offset: 40,
						Bytes:  quoteMint.Bytes(),
					},
				},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("pool not found for base mint %s and quote mint %s",
			baseMint.String(), quoteMint.String())
	}

	// Берем первый найденный пул
	poolAccount := accounts[0]

	// Получаем authority пула через PDA
	authority, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("amm_authority")},
		solana.MustPublicKeyFromBase58(RAYDIUM_V4_PROGRAM_ID),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive authority: %w", err)
	}

	// Парсим данные аккаунта
	data := poolAccount.Account.Data.GetBinary()
	if len(data) < 388 {
		return nil, fmt.Errorf("invalid pool account data length: %d", len(data))
	}

	// Извлекаем данные из бинарного представления
	// Офсеты взяты из документации Raydium и SDK
	pool := &RaydiumPool{
		ID:        poolAccount.Pubkey, // Исправлено с PublicKey на Pubkey
		Authority: authority,
		BaseMint:  baseMint,
		QuoteMint: quoteMint,
		// Извлекаем адреса vault'ов (смещения могут отличаться, нужно сверить с документацией)
		BaseVault:  solana.PublicKeyFromBytes(data[72:104]),
		QuoteVault: solana.PublicKeyFromBytes(data[104:136]),
		// Извлекаем decimals
		BaseDecimals:  uint8(data[136]),
		QuoteDecimals: uint8(data[137]),
		// Извлекаем fee в базисных пунктах (2 байта)
		DefaultFeeBps: binary.LittleEndian.Uint16(data[138:140]),
	}

	c.logger.Debug("pool info retrieved successfully",
		zap.String("poolId", pool.ID.String()),
		zap.String("baseVault", pool.BaseVault.String()),
		zap.String("quoteVault", pool.QuoteVault.String()),
		zap.Uint8("baseDecimals", pool.BaseDecimals),
		zap.Uint8("quoteDecimals", pool.QuoteDecimals),
		zap.Uint16("defaultFeeBps", pool.DefaultFeeBps),
	)

	return pool, nil
}

// GetPoolState получает текущее состояние пула
func (c *RaydiumClient) GetPoolState(pool *RaydiumPool) (*PoolState, error) {
	c.logger.Debug("getting pool state",
		zap.String("poolId", pool.ID.String()),
	)

	// Получаем данные аккаунта пула
	account, err := c.client.GetAccountInfo(context.Background(), pool.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}

	// Парсим данные в структуру состояния
	state := &PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(account.Value.Data.GetBinary()[64:72]), // пример смещения
		QuoteReserve: binary.LittleEndian.Uint64(account.Value.Data.GetBinary()[72:80]), // пример смещения
		Status:       account.Value.Data.GetBinary()[88],                                // пример смещения
	}

	return state, nil
}

// TODO:
// Для полноценной работы нужно добавить:
// Корректную сериализацию инструкций (согласно протоколу Raydium)
// Детальную обработку различных типов ошибок
// Валидацию параметров свапа
// Обработку разных версий пулов
// Расчет слиппажа и проверку лимитов

// NewSwapInstructionBuilder создает новый билдер для SwapInstruction
func NewSwapInstructionBuilder() *SwapInstruction {
	return &SwapInstruction{
		AccountMetaSlice: make(solana.AccountMetaSlice, 7), // 7 обязательных аккаунтов
	}
}

// Методы для установки параметров, следуя паттерну из SDK
func (inst *SwapInstruction) SetAmount(amount uint64) *SwapInstruction {
	inst.Amount = &amount
	return inst
}

func (inst *SwapInstruction) SetMinimumOut(minimumOut uint64) *SwapInstruction {
	inst.MinimumOut = &minimumOut
	return inst
}

// Методы для установки аккаунтов
func (inst *SwapInstruction) SetAccounts(
	pool solana.PublicKey,
	authority solana.PublicKey,
	userWallet solana.PublicKey,
	sourceToken solana.PublicKey,
	destToken solana.PublicKey,
	baseVault solana.PublicKey,
	quoteVault solana.PublicKey,
) *SwapInstruction {
	inst.AccountMetaSlice[0] = solana.Meta(pool).WRITE()
	inst.AccountMetaSlice[1] = solana.Meta(authority)
	inst.AccountMetaSlice[2] = solana.Meta(userWallet).WRITE().SIGNER()
	inst.AccountMetaSlice[3] = solana.Meta(sourceToken).WRITE()
	inst.AccountMetaSlice[4] = solana.Meta(destToken).WRITE()
	inst.AccountMetaSlice[5] = solana.Meta(baseVault).WRITE()
	inst.AccountMetaSlice[6] = solana.Meta(quoteVault).WRITE()
	return inst
}

// Validate проверяет все необходимые параметры
func (inst *SwapInstruction) Validate() error {
	if inst.Amount == nil {
		return errors.New("Amount is not set")
	}
	if inst.MinimumOut == nil {
		return errors.New("MinimumOut is not set")
	}

	// Проверка всех аккаунтов
	for i, acc := range inst.AccountMetaSlice {
		if acc == nil {
			return fmt.Errorf("account at index %d is not set", i)
		}
	}
	return nil
}

// Build создает инструкци

// ProgramID возвращает ID программы Raydium
func (i *RaydiumSwapInstruction) ProgramID() solana.PublicKey {
	return i.programID
}

// Accounts возвращает список аккаунтов
func (i *RaydiumSwapInstruction) Accounts() []*solana.AccountMeta {
	return i.accounts
}

// Data возвращает сериализованные данные инструкции
func (i *RaydiumSwapInstruction) Data() ([]byte, error) {
	return i.data, nil
}

// Build создает инструкцию
func (inst *SwapInstruction) Build() (solana.Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}

	// Сериализация данных инструкции
	data := make([]byte, 16)
	binary.LittleEndian.PutUint64(data[0:8], *inst.Amount)
	binary.LittleEndian.PutUint64(data[8:16], *inst.MinimumOut)

	// Создаем новую инструкцию, реализующую интерфейс
	instruction := &RaydiumSwapInstruction{
		programID: solana.MustPublicKeyFromBase58(RAYDIUM_V4_PROGRAM_ID),
		accounts:  inst.AccountMetaSlice,
		data:      data,
	}

	return instruction, nil
}

// CreateSwapInstructions создает инструкции для свапа
func (c *RaydiumClient) CreateSwapInstructions(params *SwapParams) ([]solana.Instruction, error) {
	if err := validateSwapParams(params); err != nil {
		return nil, err
	}

	instructions := make([]solana.Instruction, 0)

	// Добавляем инструкцию compute budget если указан приоритетный fee
	if params.PriorityFeeLamports > 0 {
		computeLimitIx, err := computebudget.NewSetComputeUnitLimitInstructionBuilder().
			SetUnits(MAX_COMPUTE_UNIT_LIMIT).
			ValidateAndBuild()
		if err != nil {
			return nil, fmt.Errorf("failed to build compute limit instruction: %w", err)
		}
		instructions = append(instructions, computeLimitIx)

		computePriceIx, err := computebudget.NewSetComputeUnitPriceInstructionBuilder().
			SetMicroLamports(params.PriorityFeeLamports).
			ValidateAndBuild()
		if err != nil {
			return nil, fmt.Errorf("failed to build compute price instruction: %w", err)
		}
		instructions = append(instructions, computePriceIx)
	}

	// Создаем инструкцию свапа
	swapIx, err := NewSwapInstructionBuilder().
		SetAmount(params.AmountIn).
		SetMinimumOut(params.MinAmountOut).
		SetAccounts(
			params.Pool.ID,
			params.Pool.Authority,
			params.UserWallet,
			params.SourceTokenAccount,
			params.DestinationTokenAccount,
			params.Pool.BaseVault,
			params.Pool.QuoteVault,
		).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build swap instruction: %w", err)
	}

	instructions = append(instructions, swapIx)

	return instructions, nil
}

// validateSwapParams проверяет входные параметры
func validateSwapParams(params *SwapParams) error {
	if params == nil {
		return errors.New("params cannot be nil")
	}
	if params.Pool == nil {
		return errors.New("pool cannot be nil")
	}
	if params.UserWallet.IsZero() {
		return errors.New("user wallet is required")
	}
	if params.SourceTokenAccount.IsZero() {
		return errors.New("source token account is required")
	}
	if params.DestinationTokenAccount.IsZero() {
		return errors.New("destination token account is required")
	}
	if params.AmountIn == 0 {
		return errors.New("amount in must be greater than 0")
	}
	if params.MinAmountOut == 0 {
		return errors.New("minimum amount out must be greater than 0")
	}
	return nil
}

// SimulateSwap выполняет симуляцию транзакции свапа
func (c *RaydiumClient) SimulateSwap(ctx context.Context, params *SwapParams) error {
	c.logger.Debug("simulating swap transaction",
		zap.String("userWallet", params.UserWallet.String()),
		zap.Uint64("amountIn", params.AmountIn),
		zap.Uint64("minAmountOut", params.MinAmountOut),
		zap.String("pool", params.Pool.ID.String()),
	)

	// Получаем инструкции для свапа
	instructions, err := c.CreateSwapInstructions(params)
	if err != nil {
		return fmt.Errorf("failed to create swap instructions: %w", err)
	}

	// Получаем последний блокхеш
	recent, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем транзакцию
	tx, err := solana.NewTransaction(
		instructions,
		recent,
		solana.TransactionPayer(params.UserWallet),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию если есть приватный ключ
	if params.PrivateKey != nil {
		tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(params.UserWallet) {
				return params.PrivateKey
			}
			return nil
		})
	}

	// Симулируем транзакцию
	simResult, err := c.client.SimulateTransaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to simulate transaction: %w", err)
	}

	// Проверяем результат симуляции
	if simResult.Err != nil {
		return fmt.Errorf("simulation failed: %s", simResult.Err)
	}

	// Анализируем логи симуляции
	for _, log := range simResult.Logs {
		c.logger.Debug("simulation log", zap.String("log", log))
	}

	c.logger.Info("swap simulation successful",
		zap.Uint64("unitsConsumed", simResult.UnitsConsumed),
		zap.String("sourceToken", params.SourceTokenAccount.String()),
		zap.String("destinationToken", params.DestinationTokenAccount.String()),
		zap.Uint64("priorityFee", params.PriorityFeeLamports),
	)

	return nil
}

// TODO:
// Основные улучшения, которые можно добавить:
// Retry логика для случаев временных сбоев
// Более детальная валидация результатов транзакции
// Механизм отмены операции по таймауту
// Сохранение истории транзакций
// Метрики выполнения свапов

// ExecuteSwap выполняет свап и возвращает signature транзакции
func (c *RaydiumClient) ExecuteSwap(params *SwapParams) (string, error) {
	c.logger.Debug("starting swap execution",
		zap.String("userWallet", params.UserWallet.String()),
		zap.Uint64("amountIn", params.AmountIn),
		zap.Uint64("minAmountOut", params.MinAmountOut),
		zap.String("pool", params.Pool.ID.String()),
		zap.Uint64("priorityFee", params.PriorityFeeLamports),
	)

	// Базовая валидация параметров
	if err := validateSwapParams(params); err != nil {
		return "", fmt.Errorf("invalid swap parameters: %w", err)
	}

	// Проверяем баланс кошелька
	ctx := context.Background()
	balance, err := c.client.GetBalance(ctx, params.UserWallet, solanarpc.CommitmentConfirmed)
	if err != nil {
		return "", fmt.Errorf("failed to get wallet balance: %w", err)
	}

	// Учитываем приоритетную комиссию и обычную комиссию транзакции
	requiredBalance := params.AmountIn + params.PriorityFeeLamports + 5000
	if balance < requiredBalance {
		return "", fmt.Errorf("insufficient balance: required %d, got %d", requiredBalance, balance)
	}

	// Создаем все необходимые инструкции для свапа
	instructions, err := c.CreateSwapInstructions(params)
	if err != nil {
		return "", fmt.Errorf("failed to create swap instructions: %w", err)
	}

	// Получаем последний блокхеш
	recent, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем транзакцию
	tx, err := solana.NewTransaction(
		instructions,
		recent,
		solana.TransactionPayer(params.UserWallet),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(params.UserWallet) {
			if params.PrivateKey != nil {
				return params.PrivateKey
			}
			return &c.privateKey
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Отправляем транзакцию
	sig, err := c.client.SendTransactionWithOpts(ctx, tx, blockchain.TransactionOptions{
		SkipPreflight:       true,
		PreflightCommitment: c.options.commitment,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	c.logger.Info("swap executed successfully",
		zap.String("signature", sig.String()),
		zap.String("explorer", fmt.Sprintf("https://explorer.solana.com/tx/%s", sig.String())),
	)

	return sig.String(), nil
}

// logUpdatedBalances вспомогательный метод для логирования балансов после свапа
func (c *RaydiumClient) logUpdatedBalances(params *SwapParams) error {
	ctx := context.Background()

	// Получаем баланс SOL
	solBalance, err := c.client.GetBalance(
		ctx,
		params.UserWallet,
		solanarpc.CommitmentConfirmed,
	)
	if err != nil {
		return fmt.Errorf("failed to get SOL balance: %w", err)
	}

	// Получаем баланс токена
	tokenBalance, err := c.client.GetTokenAccountBalance(
		ctx,
		params.DestinationTokenAccount,
		solanarpc.CommitmentConfirmed,
	)
	if err != nil {
		return fmt.Errorf("failed to get token balance: %w", err)
	}

	c.logger.Info("updated balances",
		zap.Float64("solBalance", float64(solBalance)/float64(solana.LAMPORTS_PER_SOL)),
		zap.String("tokenBalance", tokenBalance.Value.UiAmountString),
	)

	return nil
}
