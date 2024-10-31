// internal/dex/raydium/client.go
package raydium

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

const (
	maxRetries          = 3
	retryDelay          = 500 * time.Millisecond
	defaultTimeout      = 15 * time.Second
	defaultConfirmLevel = rpc.CommitmentConfirmed
)

// clientOptions содержит опции для настройки клиента
type clientOptions struct {
	timeout         time.Duration
	retries         int
	commitmentLevel rpc.CommitmentType
}

// ClientOption определяет функцию для настройки клиента
type ClientOption func(*clientOptions)

// WithTimeout устанавливает таймаут для операций
func WithTimeout(timeout time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.timeout = timeout
	}
}

// WithRetries устанавливает количество попыток
func WithRetries(retries int) ClientOption {
	return func(o *clientOptions) {
		o.retries = retries
	}
}

// WithCommitmentLevel устанавливает уровень подтверждения
func WithCommitmentLevel(level rpc.CommitmentType) ClientOption {
	return func(o *clientOptions) {
		o.commitmentLevel = level
	}
}

// getDefaultOptions возвращает опции по умолчанию
func getDefaultOptions() *clientOptions {
	return &clientOptions{
		timeout:         defaultTimeout,
		retries:         maxRetries,
		commitmentLevel: defaultConfirmLevel,
	}
}

// RaydiumClient реализует взаимодействие с Raydium DEX
type RaydiumClient struct {
	client  blockchain.Client
	logger  *zap.Logger
	options *clientOptions
}

// NewRaydiumClient создает новый экземпляр клиента с опциями
func NewRaydiumClient(client blockchain.Client, logger *zap.Logger, opts ...ClientOption) (*RaydiumClient, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	options := getDefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	return &RaydiumClient{
		client:  client,
		logger:  logger.Named("raydium-client"),
		options: options,
	}, nil
}

// GetPool получает информацию о пуле по его ID
func (c *RaydiumClient) GetPool(ctx context.Context, poolID solana.PublicKey) (*RaydiumPool, error) {
	logger := c.logger.With(zap.String("pool_id", poolID.String()))
	logger.Debug("Getting pool information")

	ctx, cancel := context.WithTimeout(ctx, c.options.timeout)
	defer cancel()

	account, err := c.client.GetAccountInfo(ctx, poolID)
	if err != nil {
		return nil, &SwapError{
			Stage:   "get_pool",
			Message: "failed to get pool account",
			Err:     err,
		}
	}

	if account.Value == nil || len(account.Value.Data.GetBinary()) == 0 {
		return nil, &SwapError{
			Stage:   "get_pool",
			Message: "pool account not found or empty",
		}
	}

	pool := &RaydiumPool{
		ID: poolID,
	}

	if err := c.decodePoolData(account.Value.Data.GetBinary(), pool); err != nil {
		return nil, &SwapError{
			Stage:   "get_pool",
			Message: "failed to decode pool data",
			Err:     err,
		}
	}

	logger.Debug("Pool information retrieved successfully")
	return pool, nil
}

// GetPoolState получает текущее состояние пула
func (c *RaydiumClient) GetPoolState(ctx context.Context, pool *RaydiumPool) (*PoolState, error) {
	logger := c.logger.With(zap.String("pool_id", pool.ID.String()))
	logger.Debug("Getting pool state")

	ctx, cancel := context.WithTimeout(ctx, c.options.timeout)
	defer cancel()

	account, err := c.client.GetAccountInfo(ctx, pool.ID)
	if err != nil {
		return nil, &SwapError{
			Stage:   "get_pool_state",
			Message: "failed to get pool account",
			Err:     err,
		}
	}

	data := account.Value.Data.GetBinary()
	if len(data) < LayoutQuoteReserveOffset+8 {
		return nil, &SwapError{
			Stage: "get_pool_state",
			Message: fmt.Sprintf("invalid pool data length: got %d, need at least %d",
				len(data), LayoutQuoteReserveOffset+8),
		}
	}

	state := &PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(data[LayoutBaseReserveOffset : LayoutBaseReserveOffset+8]),
		QuoteReserve: binary.LittleEndian.Uint64(data[LayoutQuoteReserveOffset : LayoutQuoteReserveOffset+8]),
		Status:       data[LayoutStatus],
	}

	logger.Debug("Pool state retrieved",
		zap.Uint64("base_reserve", state.BaseReserve),
		zap.Uint64("quote_reserve", state.QuoteReserve))

	return state, nil
}

// CreateSwapInstructions создает инструкции для свапа
func (c *RaydiumClient) CreateSwapInstructions(ctx context.Context, params SwapParams) ([]solana.Instruction, error) {
	logger := c.logger.With(
		zap.String("user_wallet", params.UserWallet.String()),
		zap.Uint64("amount_in", params.AmountIn),
		zap.Uint64("min_amount_out", params.MinAmountOut),
	)
	logger.Debug("Creating swap instructions")

	var instructions []solana.Instruction

	// Добавляем инструкцию compute budget
	if params.ComputeUnits > 0 {
		computeUnitInstruction := computebudget.NewSetComputeUnitLimitInstruction(
			params.ComputeUnits,
		).Build()
		instructions = append(instructions, computeUnitInstruction)
	}

	// Добавляем инструкцию priority fee если указана
	if params.PriorityFeeLamports > 0 {
		priorityFeeInstruction := computebudget.NewSetComputeUnitPriceInstruction(
			params.PriorityFeeLamports,
		).Build()
		instructions = append(instructions, priorityFeeInstruction)
	}

	// Создаем инструкцию свапа
	swapInstruction, err := c.createSwapInstruction(params)
	if err != nil {
		return nil, &SwapError{
			Stage:   "create_instructions",
			Message: "failed to create swap instruction",
			Err:     err,
		}
	}
	instructions = append(instructions, swapInstruction)

	logger.Debug("Swap instructions created successfully",
		zap.Int("instruction_count", len(instructions)))

	return instructions, nil
}

// SimulateSwap симулирует выполнение свапа
func (c *RaydiumClient) SimulateSwap(ctx context.Context, instructions []solana.Instruction) error {
	logger := c.logger.With(zap.String("method", "SimulateSwap"))
	logger.Debug("Starting swap simulation")

	recent, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return &SwapError{
			Stage:   "simulate_swap",
			Message: "failed to get recent blockhash",
			Err:     err,
		}
	}

	tx, err := solana.NewTransaction(instructions, recent)
	if err != nil {
		return &SwapError{
			Stage:   "simulate_swap",
			Message: "failed to create transaction",
			Err:     err,
		}
	}

	simulation, err := c.client.SimulateTransaction(ctx, tx)
	if err != nil {
		return &SwapError{
			Stage:   "simulate_swap",
			Message: "simulation failed",
			Err:     err,
		}
	}

	if simulation.Err != nil {
		return &SwapError{
			Stage:   "simulate_swap",
			Message: fmt.Sprintf("simulation returned error: %v", simulation.Err),
		}
	}

	logger.Debug("Swap simulation successful",
		zap.Uint64("compute_units_used", simulation.UnitsConsumed),
		zap.Strings("logs", simulation.Logs))

	return nil
}

// GetAmountOut вычисляет ожидаемый выход для свапа
func (c *RaydiumClient) GetAmountOut(pool *RaydiumPool, state *PoolState, amountIn uint64) (uint64, error) {
	if state.BaseReserve == 0 || state.QuoteReserve == 0 {
		return 0, &SwapError{
			Stage:   "get_amount_out",
			Message: "invalid pool reserves",
		}
	}

	// Учитываем комиссию пула
	amountInWithFee := float64(amountIn) * (1 - DefaultSwapFeePercent/100)

	// Используем формулу: dy = y * dx / (x + dx)
	numerator := float64(state.QuoteReserve) * amountInWithFee
	denominator := float64(state.BaseReserve) + amountInWithFee

	expectedOut := uint64(numerator / denominator)

	if expectedOut == 0 {
		return 0, &SwapError{
			Stage:   "get_amount_out",
			Message: "calculated amount out is zero",
		}
	}

	c.logger.Debug("Calculated amount out",
		zap.Uint64("amount_in", amountIn),
		zap.Uint64("amount_out", expectedOut),
		zap.Float64("fee_percent", DefaultSwapFeePercent))

	return expectedOut, nil
}

// createSwapInstruction создает инструкцию свапа
func (c *RaydiumClient) createSwapInstruction(params SwapParams) (solana.Instruction, error) {
	// Проверяем входные параметры
	if params.Pool == nil {
		return nil, &SwapError{
			Stage:   "create_swap_instruction",
			Message: "pool is required",
		}
	}

	// Создаем слайс аккаунтов для инструкции
	accounts := make(solana.AccountMetaSlice, 0)

	// Добавляем основные аккаунты
	accounts = append(accounts,
		// Token Program ID
		solana.Meta(solana.TokenProgramID),
		// AMM Program ID
		solana.Meta(params.Pool.AmmProgramID),
		// User wallet (signer)
		solana.Meta(params.UserWallet).SIGNER(),
		// Pool ID
		solana.Meta(params.Pool.ID),
		// Pool Authority
		solana.Meta(params.Pool.Authority),
		// Source token account
		solana.Meta(params.SourceTokenAccount).WRITE(),
		// Destination token account
		solana.Meta(params.DestinationTokenAccount).WRITE(),
		// Pool base vault
		solana.Meta(params.Pool.BaseVault).WRITE(),
		// Pool quote vault
		solana.Meta(params.Pool.QuoteVault).WRITE(),
	)

	// Создаем данные инструкции
	data := make([]byte, 9)
	// Команда swap (предположим, что это 1)
	data[0] = 1
	// Записываем AmountIn (8 байт, little endian)
	binary.LittleEndian.PutUint64(data[1:], params.AmountIn)

	// Создаем инструкцию
	instruction := solana.NewInstruction(
		params.Pool.AmmProgramID,
		accounts,
		data,
	)

	return instruction, nil
}

// decodePoolData декодирует данные аккаунта пула
func (c *RaydiumClient) decodePoolData(data []byte, pool *RaydiumPool) error {
	if len(data) < PoolDataSize {
		return &SwapError{
			Stage:   "decode_pool_data",
			Message: fmt.Sprintf("invalid data length: got %d, want at least %d", len(data), PoolDataSize),
		}
	}

	// Декодируем данные пула используя binary.LittleEndian
	// Предполагаем следующую структуру данных:
	offset := 0

	// Status (1 byte)
	pool.Status = data[offset]
	offset += 1

	// Nonce (1 byte)
	pool.Nonce = data[offset]
	offset += 1

	// AMM Program ID (32 bytes)
	copy(pool.AmmProgramID[:], data[offset:offset+32])
	offset += 32

	// Authority (32 bytes)
	copy(pool.Authority[:], data[offset:offset+32])
	offset += 32

	// Base Vault (32 bytes)
	copy(pool.BaseVault[:], data[offset:offset+32])
	offset += 32

	// Quote Vault (32 bytes)
	copy(pool.QuoteVault[:], data[offset:offset+32])
	offset += 32

	// Base Mint (32 bytes)
	copy(pool.BaseMint[:], data[offset:offset+32])
	offset += 32

	// Quote Mint (32 bytes)
	copy(pool.QuoteMint[:], data[offset:offset+32])
	offset += 32

	// Base Reserve (8 bytes)
	pool.BaseReserve = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Quote Reserve (8 bytes)
	pool.QuoteReserve = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Fee Rate (8 bytes)
	pool.FeeRate = binary.LittleEndian.Uint64(data[offset : offset+8])

	return nil
}

// Методы для работы с versioned transactions
// CreateVersionedSwapInstructions создает версионированную транзакцию для свапа

func (c *RaydiumClient) CreateVersionedSwapInstructions(
	ctx context.Context,
	params SwapParams,
) (*solana.Message, error) {
	logger := c.logger.With(
		zap.String("user", params.UserWallet.String()),
		zap.Uint64("amount_in", params.AmountIn),
		zap.Uint64("min_amount_out", params.MinAmountOut),
	)
	logger.Debug("Creating versioned swap instructions")

	// Получаем последний блокхэш
	recent, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем обычные инструкции свапа
	instructions, err := c.CreateSwapInstructions(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap instructions: %w", err)
	}

	// Создаем базовое сообщение
	message := &solana.Message{

		AccountKeys: []solana.PublicKey{params.UserWallet}, // Начинаем с основного кошелька
		Header: solana.MessageHeader{
			NumRequiredSignatures:       1, // Минимум одна подпись от кошелька
			NumReadonlySignedAccounts:   0,
			NumReadonlyUnsignedAccounts: 0,
		},
		RecentBlockhash: recent,
		Instructions:    make([]solana.CompiledInstruction, 0),
	}

	// Добавляем lookup таблицу, если она есть
	if params.LookupTableAccount != nil {
		lookupTable := solana.MessageAddressTableLookup{
			AccountKey: *params.LookupTableAccount,
			// Индексы для записи (writable) - обычно это токен аккаунты
			WritableIndexes: params.WritableIndexes,
			// Индексы только для чтения - обычно это программы и конфиги
			ReadonlyIndexes: params.ReadonlyIndexes,
		}
		message.AddressTableLookups = []solana.MessageAddressTableLookup{lookupTable}
		message.SetVersion(solana.MessageVersionV0) // Устанавливаем версию V0 для поддержки lookup tables
	}

	// Добавляем все инструкции
	for _, instruction := range instructions {
		compiledIx := solana.CompiledInstruction{
			ProgramIDIndex: uint16(len(message.AccountKeys)), // Индекс программы
			Data: func() []byte {
				data, err := instruction.Data()
				if err != nil {
					logger.Error("failed to get instruction data", zap.Error(err))
					return nil
				}
				return data
			}(),
			Accounts: make([]uint16, len(instruction.Accounts())),
		}

		// Добавляем ProgramID в список аккаунтов, если его там еще нет
		programID := instruction.ProgramID()
		found := false
		for i, key := range message.AccountKeys {
			if key.Equals(programID) {
				compiledIx.ProgramIDIndex = uint16(i)
				found = true
				break
			}
		}
		if !found {
			message.AccountKeys = append(message.AccountKeys, programID)
			compiledIx.ProgramIDIndex = uint16(len(message.AccountKeys) - 1)
		}

		// Обрабатываем каждый аккаунт в инструкции
		for i, acc := range instruction.Accounts() {
			found := false
			for j, key := range message.AccountKeys {
				if key.Equals(acc.PublicKey) {
					compiledIx.Accounts[i] = uint16(j)
					found = true
					break
				}
			}
			if !found {
				message.AccountKeys = append(message.AccountKeys, acc.PublicKey)
				compiledIx.Accounts[i] = uint16(len(message.AccountKeys) - 1)
			}
		}

		message.Instructions = append(message.Instructions, compiledIx)
	}

	logger.Debug("Versioned transaction message created successfully",
		zap.Int("num_instructions", len(message.Instructions)),
		zap.Int("num_accounts", len(message.AccountKeys)))

	return message, nil
}

// Методы для работы с lookup tables
// GetPoolLookupTable получает таблицу поиска адресов для пула
func (c *RaydiumClient) GetPoolLookupTable(
	ctx context.Context,
	pool *RaydiumPool,
) (*addresslookuptable.KeyedAddressLookupTable, error) {
	logger := c.logger.With(zap.String("pool_id", pool.ID.String()))
	logger.Debug("Getting pool lookup table")

	// Получаем адрес lookup таблицы из PDA
	lookupTableAddr, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("lookup_table"),
			pool.ID.Bytes(),
		},
		pool.AmmProgramID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive lookup table address: %w", err)
	}

	// Получаем RPC клиент через GetRpcClient
	rpcClient := c.client.GetRpcClient()

	// Получаем данные таблицы с помощью RPC клиента
	lookupTable, err := addresslookuptable.GetAddressLookupTable(
		ctx,
		rpcClient,
		lookupTableAddr,
	)
	if err != nil {
		jsonRpcErr, ok := err.(*jsonrpc.RPCError)
		if ok && jsonRpcErr.Code == -32001 {
			// Таблица не найдена, это нормально — возвращаем nil
			logger.Debug("Lookup table not found for pool")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get lookup table: %w", err)
	}

	// Проверяем активность таблицы
	if !lookupTable.IsActive() {
		logger.Debug("Lookup table is not active",
			zap.Uint64("deactivation_slot", lookupTable.DeactivationSlot))
		return nil, nil
	}

	requiredAddresses := []solana.PublicKey{
		pool.ID,
		pool.Authority,
		pool.BaseVault,
		pool.QuoteVault,
		pool.MarketID,
		pool.MarketBaseVault,
		pool.MarketQuoteVault,
	}

	for _, addr := range requiredAddresses {
		found := false
		for _, tableAddr := range lookupTable.Addresses {
			if tableAddr.Equals(addr) {
				found = true
				break
			}
		}
		if !found {
			logger.Debug("Required address not found in lookup table",
				zap.String("address", addr.String()))
			return nil, nil
		}
	}

	result := addresslookuptable.NewKeyedAddressLookupTable(lookupTableAddr)
	result.State = *lookupTable

	logger.Debug("Pool lookup table retrieved successfully",
		zap.Int("num_addresses", len(lookupTable.Addresses)))

	return result, nil
}
