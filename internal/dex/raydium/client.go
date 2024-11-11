// internal/dex/raydium/client.go - это пакет, который содержит в себе реализацию клиента для работы с декстером Raydium
// inernal/dex/raydium/types.go
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

// NewRaydiumClient создает новый экземпляр клиента Raydium с дефолтными настройками
func NewRaydiumClient(rpcEndpoint string, wallet solana.PrivateKey, logger *zap.Logger) (*Client, error) {
	logger = logger.Named("raydium-client")

	solClient, err := solbc.NewClient(
		[]string{rpcEndpoint},
		wallet,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana client: %w", err)
	}

	return &Client{
		client:      solClient,
		logger:      logger,
		privateKey:  wallet,
		timeout:     30 * time.Second,
		retries:     3,
		priorityFee: 1000,
		commitment:  solanarpc.CommitmentConfirmed,
		poolCache:   NewPoolCache(logger),
		api:         NewAPIService(logger),
	}, nil
}

// Добавим метод для подписания транзакций
func (c *Client) SignTransaction(tx *solana.Transaction) error {
	_, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(c.GetPublicKey()) {
			return &c.privateKey
		}
		return nil
	})
	return err
}

// Добавим метод для проверки баланса кошелька
func (c *Client) CheckWalletBalance(ctx context.Context) (uint64, error) {
	balance, err := c.client.GetBalance(
		ctx,
		c.GetPublicKey(),
		solanarpc.CommitmentConfirmed,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get wallet balance: %w", err)
	}
	return balance, nil
}

// Добавим метод для получения ATA (Associated Token Account)
func (c *Client) GetAssociatedTokenAccount(mint solana.PublicKey) (solana.PublicKey, error) {
	ata, _, err := solana.FindAssociatedTokenAddress(
		c.GetPublicKey(),
		mint,
	)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to find associated token address: %w", err)
	}
	return ata, nil
}

// GetPool получает информацию о пуле с использованием API и кэша
func (c *Client) GetPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*Pool, error) {
	c.logger.Debug("searching pool",
		zap.String("baseMint", baseMint.String()),
		zap.String("quoteMint", quoteMint.String()))

	// Сначала проверяем кэш
	if pool := c.poolCache.GetPool(baseMint, quoteMint); pool != nil {
		c.logger.Debug("pool found in cache",
			zap.String("poolId", pool.ID.String()))
		return pool, nil
	}

	// Ищем через API с retry механизмом
	var pool *Pool
	var err error

	for attempt := 0; attempt <= c.retries; attempt++ {
		// Пробуем найти по baseMint
		pool, err = c.api.GetPoolByToken(ctx, baseMint)
		if err == nil {
			if err := c.enrichPoolWithOnChainData(ctx, pool); err != nil {
				c.logger.Warn("failed to enrich pool with on-chain data",
					zap.Error(err))
				continue
			}
			break
		}

		// Если не нашли, пробуем по quoteMint
		pool, err = c.api.GetPoolByToken(ctx, quoteMint)
		if err == nil {
			if err := c.enrichPoolWithOnChainData(ctx, pool); err != nil {
				c.logger.Warn("failed to enrich pool with on-chain data",
					zap.Error(err))
				continue
			}
			break
		}

		if attempt < c.retries {
			time.Sleep(time.Second * time.Duration(attempt+1))
			continue
		}

		return nil, fmt.Errorf("failed to find pool after %d attempts: %w", c.retries+1, err)
	}

	// Сохраняем успешно найденный пул в кэш
	if err := c.poolCache.AddPool(pool); err != nil {
		c.logger.Warn("failed to cache pool",
			zap.Error(err))
	}

	return pool, nil
}

// enrichPoolWithOnChainData дополняет пул данными из блокчейна
func (c *Client) enrichPoolWithOnChainData(ctx context.Context, pool *Pool) error {
	if pool == nil {
		return errors.New("pool is nil")
	}

	account, err := c.client.GetAccountInfo(ctx, pool.ID)
	if err != nil {
		return fmt.Errorf("get account info: %w", err)
	}

	if account == nil || account.Value == nil || len(account.Value.Data.GetBinary()) < PoolAccountSize {
		return fmt.Errorf("invalid pool account data")
	}

	data := account.Value.Data.GetBinary()

	// Получаем authority через PDA
	authority, _, err := solana.FindProgramAddress(
		[][]byte{[]byte(AmmAuthorityLayout)},
		RaydiumV4ProgramID,
	)
	if err != nil {
		return fmt.Errorf("derive authority: %w", err)
	}

	// Обновляем данные пула
	pool.Authority = authority
	pool.BaseMint = solana.PublicKeyFromBytes(data[BaseMintOffset : BaseMintOffset+32])
	pool.QuoteMint = solana.PublicKeyFromBytes(data[QuoteMintOffset : QuoteMintOffset+32])
	pool.BaseVault = solana.PublicKeyFromBytes(data[BaseVaultOffset : BaseVaultOffset+32])
	pool.QuoteVault = solana.PublicKeyFromBytes(data[QuoteVaultOffset : QuoteVaultOffset+32])
	pool.BaseDecimals = data[DecimalsOffset]
	pool.QuoteDecimals = data[DecimalsOffset+1]
	pool.Version = PoolVersionV4

	// Получаем текущее состояние пула
	state := &PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(data[64:72]),
		QuoteReserve: binary.LittleEndian.Uint64(data[72:80]),
		Status:       data[88],
	}
	pool.State = *state

	return nil
}

// Вспомогательные методы
func (c *Client) GetPublicKey() solana.PublicKey {
	return c.privateKey.PublicKey()
}

func (c *Client) GetBaseClient() blockchain.Client {
	return c.client
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

// Добавляем метод для установки направления
func (inst *SwapInstruction) SetDirection(direction SwapDirection) *SwapInstruction {
	inst.Direction = &direction
	return inst
}

// Validate проверяет все необходимые параметры
func (inst *SwapInstruction) Validate() error {
	if inst.Amount == nil {
		return errors.New("amount is not set")
	}
	if inst.MinimumOut == nil {
		return errors.New("minimumOut is not set")
	}
	if inst.Direction == nil {
		return errors.New("direction is not set")
	}

	// Проверка всех аккаунтов
	for i, acc := range inst.AccountMetaSlice {
		if acc == nil {
			return fmt.Errorf("account at index %d is not set", i)
		}
	}
	return nil
}

// ProgramID возвращает ID программы Raydium
func (i *ExecutableSwapInstruction) ProgramID() solana.PublicKey {
	return i.programID
}

// Accounts возвращает список аккаунтов
func (i *ExecutableSwapInstruction) Accounts() []*solana.AccountMeta {
	return i.accounts
}

// Data возвращает сериализованные данные инструкции
func (i *ExecutableSwapInstruction) Data() ([]byte, error) {
	return i.data, nil
}

// Build создает инструкцию
func (inst *SwapInstruction) Build() (solana.Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}

	// Сериализация данных инструкции
	data := make([]byte, 17) // 16 байт для amount и minimumOut + 1 байт для direction
	binary.LittleEndian.PutUint64(data[0:8], *inst.Amount)
	binary.LittleEndian.PutUint64(data[8:16], *inst.MinimumOut)
	if *inst.Direction == SwapDirectionIn {
		data[16] = 0
	} else {
		data[16] = 1
	}

	instruction := &ExecutableSwapInstruction{
		programID: RaydiumV4ProgramID,
		accounts:  inst.AccountMetaSlice,
		data:      data,
	}

	return instruction, nil
}

// CreateSwapInstructions создает инструкции для свапа
func (c *Client) CreateSwapInstructions(params *SwapParams) ([]solana.Instruction, error) {
	if err := validateSwapParams(params); err != nil {
		return nil, err
	}

	instructions := make([]solana.Instruction, 0)

	// Добавляем инструкцию compute budget если указан приоритетный fee
	if params.PriorityFeeLamports > 0 {
		computeLimitIx, err := computebudget.NewSetComputeUnitLimitInstructionBuilder().
			SetUnits(MaxComputeUnitLimit).
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
func (c *Client) SimulateSwap(ctx context.Context, params *SwapParams) error {
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
		signatures, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(params.UserWallet) {
				return params.PrivateKey
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to sign transaction: %w", err)
		}

		// Опционально: можно добавить логирование подписей
		c.logger.Debug("transaction signed",
			zap.Int("signatures_count", len(signatures)),
			zap.String("first_signature", signatures[0].String()),
		)
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
func (c *Client) ExecuteSwap(params *SwapParams) (string, error) {
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
		PreflightCommitment: c.commitment, // используем commitment напрямую из клиента
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
