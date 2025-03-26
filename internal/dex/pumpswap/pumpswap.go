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
func (dex *DEX) effectiveMints() (baseMint, quoteMint solana.PublicKey) {
	wsol := solana.MustPublicKeyFromBase58(WSOLMint)
	// Если конфигурация указана как base = WSOL, а quote = токен,
	// то для свапа effectiveBaseMint = токен, effectiveQuoteMint = WSOL.
	if dex.config.BaseMint.Equals(wsol) && !dex.config.QuoteMint.Equals(wsol) {
		return dex.config.QuoteMint, dex.config.BaseMint
	}
	return dex.config.BaseMint, dex.config.QuoteMint
}

func (dex *DEX) getGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	dex.configMutex.RLock()
	if dex.globalConfig != nil {
		defer dex.configMutex.RUnlock()
		return dex.globalConfig, nil
	}
	dex.configMutex.RUnlock()

	globalConfigAddr, _, err := dex.config.DeriveGlobalConfigAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	globalConfigInfo, err := dex.client.GetAccountInfo(ctx, globalConfigAddr)
	if err != nil || globalConfigInfo == nil || globalConfigInfo.Value == nil {
		return nil, fmt.Errorf("failed to get global config")
	}

	config, err := ParseGlobalConfig(globalConfigInfo.Value.Data.GetBinary())
	if err != nil {
		return nil, err
	}

	dex.configMutex.Lock()
	defer dex.configMutex.Unlock()
	dex.globalConfig = config
	return config, nil
}

// prepareTokenAccounts подготавливает ATA пользователя и инструкции для их создания.
func (dex *DEX) prepareTokenAccounts(ctx context.Context, pool *PoolInfo) (*PreparedTokenAccounts, error) {
	userBaseATA, _, err := solana.FindAssociatedTokenAddress(dex.wallet.PublicKey, pool.BaseMint)
	if err != nil {
		return nil, err
	}
	userQuoteATA, _, err := solana.FindAssociatedTokenAddress(dex.wallet.PublicKey, pool.QuoteMint)
	if err != nil {
		return nil, err
	}

	createBaseATAIx := dex.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		dex.wallet.PublicKey, dex.wallet.PublicKey, pool.BaseMint)
	createQuoteATAIx := dex.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		dex.wallet.PublicKey, dex.wallet.PublicKey, pool.QuoteMint)

	globalConfig, err := dex.getGlobalConfig(ctx)
	if err != nil {
		return nil, err
	}

	protocolFeeRecipient := solana.PublicKeyFromBytes(make([]byte, 32))
	if len(globalConfig.ProtocolFeeRecipients) > 0 {
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
func (dex *DEX) preparePriorityInstructions(computeUnits uint32, priorityFeeSol string) ([]solana.Instruction, error) {
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
func (dex *DEX) buildAndSubmitTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	operation := func() (solana.Signature, error) {
		blockhash, err := dex.client.GetRecentBlockhash(ctx)
		if err != nil {
			return solana.Signature{}, backoff.Permanent(fmt.Errorf("failed to get recent blockhash: %w", err))
		}

		tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(dex.wallet.PublicKey))
		if err != nil {
			return solana.Signature{}, backoff.Permanent(fmt.Errorf("failed to create transaction: %w", err))
		}

		if err = dex.wallet.SignTransaction(tx); err != nil {
			return solana.Signature{}, backoff.Permanent(fmt.Errorf("failed to sign transaction: %w", err))
		}

		sig, txErr := dex.client.SendTransaction(ctx, tx)
		if txErr != nil {
			// Если ошибка связана с BlockhashNotFound - повторяем
			if strings.Contains(txErr.Error(), "BlockhashNotFound") {
				return solana.Signature{}, txErr
			}
			// Иначе - это постоянная ошибка
			return solana.Signature{}, backoff.Permanent(txErr)
		}

		if err = dex.client.WaitForTransactionConfirmation(ctx, sig, rpc.CommitmentConfirmed); err != nil {
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
func (dex *DEX) getTokenDecimals(ctx context.Context, mint solana.PublicKey, defaultDec uint8) uint8 {
	dec, err := dex.DetermineTokenPrecision(ctx, mint)
	if err != nil {
		dex.logger.Warn("Using default decimals", zap.Error(err), zap.String("mint", mint.String()))
		return defaultDec
	}
	return dec
}

// ExecuteSwap выполняет операцию обмена.
// Для покупки (isBuy==true) выполняется инструкция buy, для продажи – sell.
// Параметр amount (в лампортах для WSOL или в базовых единицах токена) и slippagePercent используются для расчёта ожидаемого выхода.
func (dex *DEX) ExecuteSwap(ctx context.Context, isBuy bool, amount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	pool, poolReversed, err := dex.findAndValidatePool(ctx)
	if err != nil {
		return err
	}

	// Если пул найден в обратном порядке, логируем это – для дальнейшей отладки.
	if poolReversed {
		dex.logger.Debug("Pool mint order is reversed relative to effective configuration")
		// Здесь можно было бы инвертировать операцию, но в данной логике мы уже нормализовали мины через effectiveMints().
	}

	accounts, err := dex.prepareTokenAccounts(ctx, pool)
	if err != nil {
		return err
	}

	priorityInstructions, err := dex.preparePriorityInstructions(computeUnits, priorityFeeSol)
	if err != nil {
		return err
	}

	var instructions []solana.Instruction
	instructions = append(instructions, priorityInstructions...)
	instructions = append(instructions, accounts.CreateBaseATAIx, accounts.CreateQuoteATAIx)

	// Получаем точность токенов.
	baseDecimals := dex.getTokenDecimals(ctx, pool.BaseMint, DefaultTokenDecimals)
	quoteDecimals := dex.getTokenDecimals(ctx, pool.QuoteMint, WSOLDecimals)

	dex.logger.Debug("Token decimals",
		zap.Uint8("base_decimals", baseDecimals),
		zap.Uint8("quote_decimals", quoteDecimals),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	// Подготовка общих параметров для инструкции
	swapParams := &SwapInstructionParams{
		IsBuy:                            isBuy,
		PoolAddress:                      pool.Address,
		User:                             dex.wallet.PublicKey,
		GlobalConfig:                     dex.config.GlobalConfig,
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
		EventAuthority:                   dex.config.EventAuthority,
		ProgramID:                        dex.config.ProgramID,
	}

	if isBuy {
		// При покупке мы хотим получить определённое количество токена (baseAmountOut),
		// и готовы заплатить максимум amount (WSOL).
		outputAmount, price := dex.poolManager.CalculateSwapQuote(pool, amount, false)
		minOut := uint64(float64(outputAmount) * (1.0 - slippagePercent/100.0))
		maxAmountWithBuffer := uint64(float64(amount) * (1.0 + slippagePercent/100.0))

		dex.logger.Debug("Buy swap calculation",
			zap.Uint64("input_amount", amount),
			zap.Uint64("max_amount_with_buffer", maxAmountWithBuffer),
			zap.Uint64("expected_output", outputAmount),
			zap.Uint64("min_out_amount", minOut),
			zap.Float64("price", price))

		// Установка параметров для покупки
		swapParams.Amount1 = outputAmount        // baseAmountOut для buy
		swapParams.Amount2 = maxAmountWithBuffer // maxQuoteAmountIn для buy
	} else {
		// Продажа: продаём токен (base) за WSOL.
		expectedOutput, price := dex.poolManager.CalculateSwapQuote(pool, amount, false)
		minOut := uint64(float64(expectedOutput) * (1.0 - slippagePercent/100.0))

		dex.logger.Debug("Sell swap calculation",
			zap.Uint64("input_amount", amount),
			zap.Uint64("expected_output", expectedOutput),
			zap.Uint64("min_out_amount", minOut),
			zap.Float64("price", price))

		// Установка параметров для продажи
		swapParams.Amount1 = amount // baseAmountIn для sell
		swapParams.Amount2 = minOut // minQuoteAmountOut для sell
	}

	// Создание инструкции для свапа с использованием новой функции
	swapIx := createSwapInstruction(swapParams)

	instructions = append(instructions, swapIx)
	sig, err := dex.buildAndSubmitTransaction(ctx, instructions)
	if err != nil {
		// Проверяем ошибку на тип ExceededSlippage и логируем более подробно
		if strings.Contains(err.Error(), "ExceededSlippage") ||
			strings.Contains(err.Error(), "0x1774") ||
			strings.Contains(err.Error(), "6004") {
			dex.logger.Warn("Exceeded slippage error - try increasing slippage percentage",
				zap.Float64("current_slippage_percent", slippagePercent),
				zap.Uint64("amount", amount),
				zap.Error(err))
			return fmt.Errorf("slippage exceeded: transaction requires more funds than maximum specified: %w", err)
		}
		return err
	}

	dex.logger.Info("Swap executed",
		zap.String("signature", sig.String()),
		zap.Bool("is_buy", isBuy),
		zap.Uint64("amount", amount),
		zap.Float64("slippage_percent", slippagePercent))
	return nil
}

// ExecuteSell выполняет операцию продажи токена за WSOL.
func (dex *DEX) ExecuteSell(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	return dex.ExecuteSwap(ctx, false, tokenAmount, slippagePercent, priorityFeeSol, computeUnits)
}

// GetTokenPrice вычисляет цену токена по соотношению резервов пула.
func (dex *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Здесь мы считаем, что tokenMint должен соответствовать effectiveBaseMint.
	effBase, effQuote := dex.effectiveMints()
	mint, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}
	if !mint.Equals(effBase) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s", effBase.String(), mint.String())
	}
	pool, err := dex.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 3, 1*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to find pool: %w", err)
	}
	var price float64
	if pool.BaseReserves > 0 && pool.QuoteReserves > 0 {
		solDecimals := uint8(WSOLDecimals)
		tokenDecimals := dex.getTokenDecimals(ctx, pool.BaseMint, DefaultTokenDecimals)
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
func (dex *DEX) DetermineTokenPrecision(ctx context.Context, mint solana.PublicKey) (uint8, error) {
	var mintInfo token.Mint
	err := dex.client.GetAccountDataInto(ctx, mint, &mintInfo)
	if err != nil {
		return 0, fmt.Errorf("failed to get mint info: %w", err)
	}

	return mintInfo.Decimals, nil
}
