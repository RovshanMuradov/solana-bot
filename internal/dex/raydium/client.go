// internal/dex/raydium/client.go - это пакет, который содержит в себе реализацию клиента для работы с декстером Raydium
package raydium

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

const (
	RAYDIUM_V4_PROGRAM_ID = "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8"
)

type RaydiumClient struct {
	client  blockchain.Client
	logger  *zap.Logger
	options *clientOptions // Базовые настройки таймаутов и retry
}
type clientOptions struct {
	timeout     time.Duration      // Таймаут для операций
	retries     int                // Количество повторных попыток
	priorityFee uint64             // Приоритетная комиссия в лампортах
	commitment  rpc.CommitmentType // Уровень подтверждения транзакций
}

// Вспомогательные структуры для инструкций
type ComputeBudgetInstruction struct {
	Units         uint32
	MicroLamports uint64
}

type SwapInstruction struct {
	Amount     uint64
	MinimumOut uint64
}

// NewRaydiumClient создает новый экземпляр клиента Raydium
func NewRaydiumClient(rpcEndpoint string, wallet solana.PrivateKey, logger *zap.Logger) *RaydiumClient {
	// Инициализация с базовыми настройками

	opts := &clientOptions{
		timeout:     30 * time.Second,
		retries:     3,
		priorityFee: 1000, // базовое значение в лампортах
		commitment:  rpc.CommitmentConfirmed,
	}

	client := blockchain.NewSolanaClient(rpcEndpoint, wallet)

	return &RaydiumClient{
		client:  client,
		logger:  logger,
		options: opts,
	}
}

// GetPool получает информацию о пуле по базовому и котируемому токенам
func (c *RaydiumClient) GetPool(baseMint, quoteMint solana.PublicKey) (*RaydiumPool, error) {
	// Получение информации о пуле

	c.logger.Debug("getting raydium pool info",
		zap.String("baseMint", baseMint.String()),
		zap.String("quoteMint", quoteMint.String()),
	)

	// Получаем программные аккаунты по фильтрам
	accounts, err := c.client.GetProgramAccounts(
		solana.MustPublicKeyFromBase58(RAYDIUM_V4_PROGRAM_ID),
		rpc.GetProgramAccountsOpts{
			Filters: []rpc.RPCFilter{
				{
					DataSize: 388, // размер аккаунта пула v4
				},
				{
					Memcmp: &rpc.RPCFilterMemcmp{
						Offset: 8, // смещение для baseMint
						Bytes:  baseMint.Bytes(),
					},
				},
				{
					Memcmp: &rpc.RPCFilterMemcmp{
						Offset: 40, // смещение для quoteMint
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
		return nil, fmt.Errorf("pool not found")
	}

	// Берем первый найденный пул
	poolAccount := accounts[0]

	// Парсим данные аккаунта в структуру пула
	pool := &RaydiumPool{
		ID:        poolAccount.PublicKey,
		BaseMint:  baseMint,
		QuoteMint: quoteMint,
		// ... заполнение остальных полей из данных аккаунта
	}

	return pool, nil
}

// GetPoolState получает текущее состояние пула
func (c *RaydiumClient) GetPoolState(pool *RaydiumPool) (*PoolState, error) {
	c.logger.Debug("getting pool state",
		zap.String("poolId", pool.ID.String()),
	)

	// Получаем данные аккаунта пула
	account, err := c.client.GetAccountInfo(pool.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}

	// Парсим данные в структуру состояния
	state := &PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(account.Data[64:72]), // пример смещения
		QuoteReserve: binary.LittleEndian.Uint64(account.Data[72:80]), // пример смещения
		Status:       account.Data[88],                                // пример смещения
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

// CreateSwapInstructions создает набор инструкций для свапа
func (c *RaydiumClient) CreateSwapInstructions(params *SwapParams) ([]solana.Instruction, error) {
	c.logger.Debug("creating swap instructions",
		zap.String("userWallet", params.UserWallet.String()),
		zap.Uint64("amountIn", params.AmountIn),
		zap.Uint64("minAmountOut", params.MinAmountOut),
	)

	// Создаем базовый массив инструкций
	instructions := make([]solana.Instruction, 0)

	// Создаем инструкцию для установки приоритета комиссии
	if params.PriorityFeeLamports > 0 {
		computeBudgetIx := solana.NewInstruction(
			solana.ComputeBudget,
			&ComputeBudgetInstruction{
				Units:         300000, // базовые compute units как в typescript
				MicroLamports: params.PriorityFeeLamports,
			},
		)
		instructions = append(instructions, computeBudgetIx)
	}

	// Создаем основную инструкцию свапа
	swapIx := solana.NewInstruction(
		solana.MustPublicKeyFromBase58(RAYDIUM_V4_PROGRAM_ID),
		&SwapInstruction{
			Amount:     params.AmountIn,
			MinimumOut: params.MinAmountOut,
		},
		// Добавляем необходимые аккаунты
		[]solana.AccountMeta{
			{PublicKey: params.Pool.ID, IsWritable: true, IsSigner: false},
			{PublicKey: params.Pool.Authority, IsWritable: false, IsSigner: false},
			{PublicKey: params.UserWallet, IsWritable: true, IsSigner: true},
			{PublicKey: params.SourceTokenAccount, IsWritable: true, IsSigner: false},
			{PublicKey: params.DestinationTokenAccount, IsWritable: true, IsSigner: false},
			{PublicKey: params.Pool.BaseVault, IsWritable: true, IsSigner: false},
			{PublicKey: params.Pool.QuoteVault, IsWritable: true, IsSigner: false},
			// Добавляем остальные необходимые аккаунты
		},
	)
	instructions = append(instructions, swapIx)

	return instructions, nil
}

// SimulateSwap выполняет симуляцию транзакции свапа
func (c *RaydiumClient) SimulateSwap(params *SwapParams) error {
	c.logger.Debug("simulating swap transaction")

	// Получаем инструкции для свапа
	instructions, err := c.CreateSwapInstructions(params)
	if err != nil {
		return fmt.Errorf("failed to create swap instructions: %w", err)
	}

	// Создаем транзакцию
	recent, err := c.client.GetRecentBlockhash()
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	tx := solana.NewTransaction(
		instructions,
		recent.Value.Blockhash,
		solana.TransactionPayer(params.UserWallet),
	)

	// Симулируем транзакцию
	sim, err := c.client.SimulateTransaction(tx, &rpc.SimulateTransactionOpts{
		SigVerify:              false,
		Commitment:             c.options.commitment,
		ReplaceRecentBlockhash: true,
	})
	if err != nil {
		return fmt.Errorf("failed to simulate transaction: %w", err)
	}

	// Проверяем результат симуляции
	if sim.Value.Err != nil {
		return fmt.Errorf("simulation failed: %s", sim.Value.Err)
	}

	// Анализируем логи симуляции
	for _, log := range sim.Value.Logs {
		c.logger.Debug("simulation log", zap.String("log", log))
	}

	c.logger.Info("swap simulation successful",
		zap.Uint64("unitsConsumed", sim.Value.UnitsConsumed),
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
	c.logger.Debug("executing swap",
		zap.String("userWallet", params.UserWallet.String()),
		zap.Uint64("amountIn", params.AmountIn),
		zap.Uint64("minAmountOut", params.MinAmountOut),
	)

	// Сначала симулируем транзакцию
	if err := c.SimulateSwap(params); err != nil {
		return "", fmt.Errorf("swap simulation failed: %w", err)
	}

	// Получаем инструкции для свапа
	instructions, err := c.CreateSwapInstructions(params)
	if err != nil {
		return "", fmt.Errorf("failed to create swap instructions: %w", err)
	}

	// Получаем последний блокхэш
	recent, err := c.client.GetRecentBlockhash()
	if err != nil {
		return "", fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем транзакцию
	tx := solana.NewTransaction(
		instructions,
		recent.Value.Blockhash,
		solana.TransactionPayer(params.UserWallet),
	)

	// Подписываем транзакцию
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(params.UserWallet) {
			return &c.client.PrivateKey
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Отправляем транзакцию
	sig, err := c.client.SendTransaction(tx, &rpc.SendTransactionOpts{
		SkipPreflight:       true,
		PreflightCommitment: c.options.commitment,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	// Ждем подтверждения транзакции
	confirmationStrategy := rpc.TransactionConfirmationStrategy{
		Signature:            sig,
		Commitment:           c.options.commitment,
		LastValidBlockHeight: recent.Value.LastValidBlockHeight,
	}

	startTime := time.Now()
	confirmation, err := c.client.ConfirmTransaction(
		confirmationStrategy,
		&rpc.ConfirmTransactionOpts{
			MaxRetries: c.options.retries,
			Timeout:    c.options.timeout,
		},
	)
	if err != nil {
		return sig, fmt.Errorf("failed to confirm transaction: %w", err)
	}

	// Проверяем статус подтверждения
	if confirmation.Value.Err != nil {
		return sig, fmt.Errorf("transaction confirmed with error: %v", confirmation.Value.Err)
	}

	c.logger.Info("swap executed successfully",
		zap.String("signature", sig),
		zap.Duration("duration", time.Since(startTime)),
	)

	// Опционально: получаем и логируем новые балансы
	if err := c.logUpdatedBalances(params); err != nil {
		c.logger.Warn("failed to get updated balances", zap.Error(err))
	}

	return sig, nil
}

// logUpdatedBalances вспомогательный метод для логирования балансов после свапа
func (c *RaydiumClient) logUpdatedBalances(params *SwapParams) error {
	// Получаем баланс SOL
	solBalance, err := c.client.GetBalance(
		params.UserWallet,
		rpc.CommitmentConfirmed,
	)
	if err != nil {
		return fmt.Errorf("failed to get SOL balance: %w", err)
	}

	// Получаем баланс токена
	tokenBalance, err := c.client.GetTokenAccountBalance(
		params.DestinationTokenAccount,
		rpc.CommitmentConfirmed,
	)
	if err != nil {
		return fmt.Errorf("failed to get token balance: %w", err)
	}

	c.logger.Info("updated balances",
		zap.Float64("solBalance", float64(solBalance.Value)/float64(solana.LAMPORTS_PER_SOL)),
		zap.String("tokenBalance", tokenBalance.Value.UiAmountString),
	)

	return nil
}
