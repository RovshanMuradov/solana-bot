// =============================
// File: internal/dex/pumpswap/pumpswap.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff/v5"
	"github.com/gagliardetto/solana-go/programs/token"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

const (
	// Адрес WSOL токена
	WSOLMint = "So11111111111111111111111111111111111111112"

	// Decimals по умолчанию
	DefaultTokenDecimals = 6
	WSOLDecimals         = 9
)

type PreparedTokenAccounts struct {
	UserBaseATA             solana.PublicKey
	UserQuoteATA            solana.PublicKey
	ProtocolFeeRecipientATA solana.PublicKey
	ProtocolFeeRecipient    solana.PublicKey
	CreateBaseATAIx         solana.Instruction
	CreateQuoteATAIx        solana.Instruction
}

// DEX реализует операции для PumpSwap.
type DEX struct {
	client      *solbc.Client
	wallet      *wallet.Wallet
	logger      *zap.Logger
	config      *Config
	poolManager *PoolManager
	rpc         *rpc.Client

	// Новые поля
	globalConfig *GlobalConfig
	configMutex  sync.RWMutex
}

// SwapAmounts содержит результаты расчёта параметров свапа
type SwapAmounts struct {
	BaseAmount  uint64  // Сумма базовой валюты
	QuoteAmount uint64  // Сумма котируемой валюты
	Price       float64 // Расчётная цена
}

// Определяем SwapParams локально в пакете pumpswap
type SwapParams struct {
	IsBuy           bool
	Amount          uint64
	SlippagePercent float64
	PriorityFeeSol  string
	ComputeUnits    uint32
}

// SlippageExceededError представляет ошибку превышения проскальзывания
type SlippageExceededError struct {
	SlippagePercent float64
	Amount          uint64
	OriginalError   error
}

// NewDEX создаёт новый экземпляр DEX для PumpSwap.
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, monitorInterval string) (*DEX, error) {
	if client == nil || w == nil || logger == nil || config == nil {
		return nil, fmt.Errorf("client, wallet, logger и config не могут быть nil")
	}
	if monitorInterval != "" {
		config.MonitorInterval = monitorInterval
	}
	return &DEX{
		client:      client,
		wallet:      w,
		logger:      logger,
		config:      config,
		poolManager: NewPoolManager(client, logger),
	}, nil
}

// effectiveMints возвращает эффективные значения базового и квотного минтов для свапа.
// Для операции swap WSOL→токен мы хотим, чтобы базовый токен был именно токеном (покупаемым),
// а квотный – WSOL. Если в конфигурации указано обратное (base = WSOL, quote = токен),
// то мы инвертируем их.
func (d *DEX) effectiveMints() (baseMint, quoteMint solana.PublicKey) {
	wsol := solana.MustPublicKeyFromBase58(WSOLMint)
	// Если конфигурация указана как base = WSOL, а quote = токен,
	// то для свапа effectiveBaseMint = токен, effectiveQuoteMint = WSOL.
	if d.config.BaseMint.Equals(wsol) && !d.config.QuoteMint.Equals(wsol) {
		return d.config.QuoteMint, d.config.BaseMint
	}
	return d.config.BaseMint, d.config.QuoteMint
}

func (d *DEX) getGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	// First check with read lock
	d.configMutex.RLock()
	if d.globalConfig != nil {
		config := d.globalConfig
		d.configMutex.RUnlock()
		return config, nil
	}
	d.configMutex.RUnlock()

	// If not found, use write lock for the entire fetch-and-set operation
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	// Double-check after acquiring write lock
	if d.globalConfig != nil {
		return d.globalConfig, nil
	}

	globalConfigAddr, _, err := d.config.DeriveGlobalConfigAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	globalConfigInfo, err := d.client.GetAccountInfo(ctx, globalConfigAddr)
	if err != nil || globalConfigInfo == nil || globalConfigInfo.Value == nil {
		return nil, fmt.Errorf("failed to get global config: %w", err)
	}

	config, err := ParseGlobalConfig(globalConfigInfo.Value.Data.GetBinary())
	if err != nil {
		return nil, err
	}

	d.globalConfig = config
	return config, nil
}

// prepareTokenAccounts подготавливает ATA пользователя и инструкции для их создания.
func (d *DEX) prepareTokenAccounts(ctx context.Context, pool *PoolInfo) (*PreparedTokenAccounts, error) {
	userBaseATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, pool.BaseMint)
	if err != nil {
		return nil, err
	}

	userQuoteATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, pool.QuoteMint)
	if err != nil {
		return nil, err
	}

	createBaseATAIx := d.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		d.wallet.PublicKey, d.wallet.PublicKey, pool.BaseMint)
	createQuoteATAIx := d.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		d.wallet.PublicKey, d.wallet.PublicKey, pool.QuoteMint)

	globalConfig, err := d.getGlobalConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize with zero key and check if first recipient is non-zero
	protocolFeeRecipient := solana.PublicKeyFromBytes(make([]byte, 32))
	if !globalConfig.ProtocolFeeRecipients[0].IsZero() {
		protocolFeeRecipient = globalConfig.ProtocolFeeRecipients[0]
	}

	protocolFeeRecipientATA, _, err := solana.FindAssociatedTokenAddress(
		protocolFeeRecipient,
		pool.QuoteMint,
	)
	if err != nil {
		return nil, err
	}

	return &PreparedTokenAccounts{
		UserBaseATA:             userBaseATA,
		UserQuoteATA:            userQuoteATA,
		ProtocolFeeRecipientATA: protocolFeeRecipientATA,
		ProtocolFeeRecipient:    protocolFeeRecipient,
		CreateBaseATAIx:         createBaseATAIx,
		CreateQuoteATAIx:        createQuoteATAIx,
	}, nil
}

// preparePriorityInstructions подготавливает инструкции для установки лимита и цены вычислительных единиц.
func (d *DEX) preparePriorityInstructions(computeUnits uint32, priorityFeeSol string) ([]solana.Instruction, error) {
	var instructions []solana.Instruction
	if computeUnits > 0 {
		instructions = append(instructions,
			computebudget.NewSetComputeUnitLimitInstruction(computeUnits).Build())
	}
	if priorityFeeSol != "" {
		fee, err := strconv.ParseFloat(priorityFeeSol, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid priority fee: %w", err)
		}
		// Перевод SOL в лампорты (1 SOL = 1e9 лампортов)
		feeLamports := uint64(fee * 1e9)
		if feeLamports > 0 {
			instructions = append(instructions,
				computebudget.NewSetComputeUnitPriceInstruction(feeLamports).Build())
		}
	}
	return instructions, nil
}

// buildAndSubmitTransaction строит, подписывает и отправляет транзакцию.
func (d *DEX) buildAndSubmitTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	operation := func() (solana.Signature, error) {
		blockhash, err := d.client.GetRecentBlockhash(ctx)
		if err != nil {
			return solana.Signature{}, backoff.Permanent(fmt.Errorf("failed to get recent blockhash: %w", err))
		}

		tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(d.wallet.PublicKey))
		if err != nil {
			return solana.Signature{}, backoff.Permanent(fmt.Errorf("failed to create transaction: %w", err))
		}

		if err = d.wallet.SignTransaction(tx); err != nil {
			return solana.Signature{}, backoff.Permanent(fmt.Errorf("failed to sign transaction: %w", err))
		}

		sig, txErr := d.client.SendTransaction(ctx, tx)
		if txErr != nil {
			// Если ошибка связана с BlockhashNotFound - повторяем
			if strings.Contains(txErr.Error(), "BlockhashNotFound") {
				return solana.Signature{}, txErr
			}
			// Иначе - это постоянная ошибка
			return solana.Signature{}, backoff.Permanent(txErr)
		}

		if err = d.client.WaitForTransactionConfirmation(ctx, sig, rpc.CommitmentConfirmed); err != nil {
			return sig, err // Возвращаем сигнатуру вместе с ошибкой
		}

		return sig, nil
	}

	// Используем правильный API для backoff v5
	sig, err := backoff.Retry(
		ctx,
		operation,
		backoff.WithBackOff(backoff.NewExponentialBackOff()),
		backoff.WithMaxElapsedTime(15*time.Second),
	)

	return sig, err
}

// getTokenDecimals получает количество десятичных знаков для токена.
func (d *DEX) getTokenDecimals(ctx context.Context, mint solana.PublicKey, defaultDec uint8) uint8 {
	dec, err := d.DetermineTokenPrecision(ctx, mint)
	if err != nil {
		d.logger.Warn("Using default decimals", zap.Error(err), zap.String("mint", mint.String()))
		return defaultDec
	}
	return dec
}

// calculateSwapAmounts вычисляет параметры для операции свапа в зависимости от типа операции (покупка/продажа)
func (d *DEX) calculateSwapAmounts(
	pool *PoolInfo,
	isBuy bool,
	amount uint64,
	slippagePercent float64,
) *SwapAmounts {
	if isBuy {
		// При покупке мы хотим получить определённое количество токена (baseAmountOut),
		// и готовы заплатить максимум amount (WSOL).
		outputAmount, price := d.poolManager.CalculateSwapQuote(pool, amount, false)
		minOut := uint64(float64(outputAmount) * (1.0 - slippagePercent/100.0))
		maxAmountWithBuffer := uint64(float64(amount) * (1.0 + slippagePercent/100.0))

		d.logger.Debug("Buy swap calculation",
			zap.Uint64("input_amount", amount),
			zap.Uint64("max_amount_with_buffer", maxAmountWithBuffer),
			zap.Uint64("expected_output", outputAmount),
			zap.Uint64("min_out_amount", minOut),
			zap.Float64("price", price))

		return &SwapAmounts{
			BaseAmount:  outputAmount,
			QuoteAmount: maxAmountWithBuffer,
			Price:       price,
		}
	} else {
		// Продажа: продаём токен (base) за WSOL.
		expectedOutput, price := d.poolManager.CalculateSwapQuote(pool, amount, false)
		minOut := uint64(float64(expectedOutput) * (1.0 - slippagePercent/100.0))

		d.logger.Debug("Sell swap calculation",
			zap.Uint64("input_amount", amount),
			zap.Uint64("expected_output", expectedOutput),
			zap.Uint64("min_out_amount", minOut),
			zap.Float64("price", price))

		return &SwapAmounts{
			BaseAmount:  amount,
			QuoteAmount: minOut,
			Price:       price,
		}
	}
}

// prepareSwapParams создает структуру параметров для инструкции свапа
func (d *DEX) prepareSwapParams(
	pool *PoolInfo,
	accounts *PreparedTokenAccounts,
	isBuy bool,
	baseAmount uint64,
	quoteAmount uint64,
) *SwapInstructionParams {
	return &SwapInstructionParams{
		IsBuy:                            isBuy,
		PoolAddress:                      pool.Address,
		User:                             d.wallet.PublicKey,
		GlobalConfig:                     d.config.GlobalConfig,
		BaseMint:                         pool.BaseMint,
		QuoteMint:                        pool.QuoteMint,
		UserBaseTokenAccount:             accounts.UserBaseATA,
		UserQuoteTokenAccount:            accounts.UserQuoteATA,
		PoolBaseTokenAccount:             pool.PoolBaseTokenAccount,
		PoolQuoteTokenAccount:            pool.PoolQuoteTokenAccount,
		ProtocolFeeRecipient:             accounts.ProtocolFeeRecipient,
		ProtocolFeeRecipientTokenAccount: accounts.ProtocolFeeRecipientATA,
		BaseTokenProgram:                 TokenProgramID,
		QuoteTokenProgram:                TokenProgramID,
		EventAuthority:                   d.config.EventAuthority,
		ProgramID:                        d.config.ProgramID,
		Amount1:                          baseAmount,
		Amount2:                          quoteAmount,
	}
}

// buildSwapTransaction создает полный список инструкций для транзакции свапа
func (d *DEX) buildSwapTransaction(
	pool *PoolInfo,
	accounts *PreparedTokenAccounts,
	isBuy bool,
	baseAmount, quoteAmount uint64,
	priorityInstructions []solana.Instruction,
) []solana.Instruction {
	var instructions []solana.Instruction
	instructions = append(instructions, priorityInstructions...)
	instructions = append(instructions, accounts.CreateBaseATAIx, accounts.CreateQuoteATAIx)

	swapParams := d.prepareSwapParams(pool, accounts, isBuy, baseAmount, quoteAmount)
	swapIx := createSwapInstruction(swapParams)
	instructions = append(instructions, swapIx)

	return instructions
}

// Константы для кодов ошибок Solana
const (
	SlippageExceededCode    = "0x1774"
	SlippageExceededCodeInt = 6004
)

// IsSlippageExceededError определяет, является ли ошибка ошибкой превышения проскальзывания
func IsSlippageExceededError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "ExceededSlippage") ||
		strings.Contains(err.Error(), SlippageExceededCode) ||
		strings.Contains(err.Error(), strconv.Itoa(SlippageExceededCodeInt)))
}

func (e *SlippageExceededError) Error() string {
	return fmt.Sprintf("slippage exceeded: transaction requires more funds than maximum specified (%f%%): %v",
		e.SlippagePercent, e.OriginalError)
}

func (e *SlippageExceededError) Unwrap() error {
	return e.OriginalError
}

// handleSwapError обрабатывает специфичные ошибки операции свапа
func (d *DEX) handleSwapError(err error, params SwapParams) error {
	if IsSlippageExceededError(err) {
		d.logger.Warn("Exceeded slippage error - try increasing slippage percentage",
			zap.Float64("current_slippage_percent", params.SlippagePercent),
			zap.Uint64("amount", params.Amount),
			zap.Error(err))
		return &SlippageExceededError{
			SlippagePercent: params.SlippagePercent,
			Amount:          params.Amount,
			OriginalError:   err,
		}
	}
	return err
}

// ExecuteSwap выполняет операцию обмена на DEX.
//
// Принимает SwapParams, содержащий следующие параметры:
// - IsBuy: для покупки (true) выполняется инструкция buy, для продажи (false) - sell
// - Amount: количество токенов в базовых единицах (для продажи) или SOL в лампортах (для покупки)
// - SlippagePercent: допустимое проскальзывание в процентах (0-100)
// - PriorityFeeSol: приоритетная комиссия в SOL (строковое представление)
// - ComputeUnits: лимит вычислительных единиц для транзакции
//
// Возвращает ошибку в случае неудачи, включая специализированную SlippageExceededError
// при превышении допустимого проскальзывания.
func (d *DEX) ExecuteSwap(ctx context.Context, params SwapParams) error {
	pool, poolReversed, err := d.findAndValidatePool(ctx)
	if err != nil {
		return err
	}

	// Если пул найден в обратном порядке, логируем это
	if poolReversed {
		d.logger.Debug("Pool mint order is reversed relative to effective configuration")
	}

	accounts, err := d.prepareTokenAccounts(ctx, pool)
	if err != nil {
		return err
	}

	priorityInstructions, err := d.preparePriorityInstructions(params.ComputeUnits, params.PriorityFeeSol)
	if err != nil {
		return err
	}

	// Получаем точность токенов
	baseDecimals := d.getTokenDecimals(ctx, pool.BaseMint, DefaultTokenDecimals)
	quoteDecimals := d.getTokenDecimals(ctx, pool.QuoteMint, WSOLDecimals)

	d.logger.Debug("Token decimals",
		zap.Uint8("base_decimals", baseDecimals),
		zap.Uint8("quote_decimals", quoteDecimals),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	// Вычисляем параметры для свапа
	amounts := d.calculateSwapAmounts(pool, params.IsBuy, params.Amount, params.SlippagePercent)

	// Собираем инструкции для транзакции
	instructions := d.buildSwapTransaction(pool, accounts, params.IsBuy, amounts.BaseAmount, amounts.QuoteAmount, priorityInstructions)

	// Отправляем транзакцию
	sig, err := d.buildAndSubmitTransaction(ctx, instructions)
	if err != nil {
		return d.handleSwapError(err, params)
	}

	d.logger.Info("Swap executed",
		zap.String("signature", sig.String()),
		zap.Bool("is_buy", params.IsBuy),
		zap.Uint64("amount", params.Amount),
		zap.Float64("slippage_percent", params.SlippagePercent))
	return nil
}

// ExecuteSell выполняет операцию продажи токена за WSOL
func (d *DEX) ExecuteSell(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	params := SwapParams{
		IsBuy:           false,
		Amount:          tokenAmount,
		SlippagePercent: slippagePercent,
		PriorityFeeSol:  priorityFeeSol,
		ComputeUnits:    computeUnits,
	}
	return d.ExecuteSwap(ctx, params)
}

// GetTokenPrice вычисляет цену токена по соотношению резервов пула.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Здесь мы считаем, что tokenMint должен соответствовать effectiveBaseMint.
	effBase, effQuote := d.effectiveMints()
	mint, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}
	if !mint.Equals(effBase) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s", effBase.String(), mint.String())
	}
	pool, err := d.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 3, 1*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to find pool: %w", err)
	}
	var price float64
	if pool.BaseReserves > 0 && pool.QuoteReserves > 0 {
		solDecimals := uint8(WSOLDecimals)
		tokenDecimals := d.getTokenDecimals(ctx, pool.BaseMint, DefaultTokenDecimals)
		baseReserves := new(big.Float).SetUint64(pool.BaseReserves)
		quoteReserves := new(big.Float).SetUint64(pool.QuoteReserves)
		ratio := new(big.Float).Quo(baseReserves, quoteReserves)
		adjustment := math.Pow10(int(tokenDecimals)) / math.Pow10(int(solDecimals))
		adjustedRatio := new(big.Float).Mul(ratio, big.NewFloat(adjustment))
		price, _ = adjustedRatio.Float64()
	}
	return price, nil
}

// DetermineTokenPrecision получает количество десятичных знаков для данного токена.
func (d *DEX) DetermineTokenPrecision(ctx context.Context, mint solana.PublicKey) (uint8, error) {
	var mintInfo token.Mint
	err := d.client.GetAccountDataInto(ctx, mint, &mintInfo)
	if err != nil {
		return 0, fmt.Errorf("failed to get mint info: %w", err)
	}

	return mintInfo.Decimals, nil
}
