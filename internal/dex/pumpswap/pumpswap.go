package pumpswap

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// DEX реализует операции для PumpSwap.
type DEX struct {
	client      *solbc.Client
	wallet      *wallet.Wallet
	logger      *zap.Logger
	config      *Config
	poolManager *PoolManager
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
	wsol := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	// Если конфигурация указана как base = WSOL, а quote = токен,
	// то для свапа effectiveBaseMint = токен, effectiveQuoteMint = WSOL.
	if dex.config.BaseMint.Equals(wsol) && !dex.config.QuoteMint.Equals(wsol) {
		return dex.config.QuoteMint, dex.config.BaseMint
	}
	return dex.config.BaseMint, dex.config.QuoteMint
}

// findAndValidatePool ищет пул для эффективной пары (baseMint, quoteMint) и проверяет, что
// найденный пул соответствует ожидаемым значениям (base mint должен совпадать).
func (dex *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	// Получаем эффективные значения минтов для свапа.
	effBase, effQuote := dex.effectiveMints()

	// Ищем пул с заданной парой с повторами.
	pool, err := dex.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 5, 2*time.Second)
	if err != nil {
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	// Обновляем конфигурацию (адрес пула и LP-токена).
	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	dex.logger.Debug("Found pool details",
		zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	// Если пул найден в обратном порядке, вернём флаг poolMintReversed = true.
	poolMintReversed := false
	if !pool.BaseMint.Equals(effBase) {
		poolMintReversed = true
	}

	return pool, poolMintReversed, nil
}

// prepareTokenAccounts подготавливает ATA пользователя и инструкции для их создания.
func (dex *DEX) prepareTokenAccounts(ctx context.Context, pool *PoolInfo) (
	userBaseATA, userQuoteATA, protocolFeeRecipientATA, protocolFeeRecipient solana.PublicKey,
	createBaseATAIx, createQuoteATAIx solana.Instruction,
	err error,
) {
	userBaseATA, _, err = solana.FindAssociatedTokenAddress(dex.wallet.PublicKey, pool.BaseMint)
	if err != nil {
		return
	}
	userQuoteATA, _, err = solana.FindAssociatedTokenAddress(dex.wallet.PublicKey, pool.QuoteMint)
	if err != nil {
		return
	}

	createBaseATAIx = dex.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		dex.wallet.PublicKey, dex.wallet.PublicKey, pool.BaseMint)
	createQuoteATAIx = dex.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		dex.wallet.PublicKey, dex.wallet.PublicKey, pool.QuoteMint)

	globalConfigAddr, _, err := dex.config.DeriveGlobalConfigAddress()
	if err != nil {
		return
	}
	globalConfigInfo, err := dex.client.GetAccountInfo(ctx, globalConfigAddr)
	if err != nil || globalConfigInfo == nil || globalConfigInfo.Value == nil {
		err = fmt.Errorf("failed to get global config")
		return
	}
	globalConfig, err := ParseGlobalConfig(globalConfigInfo.Value.Data.GetBinary())
	if err != nil {
		return
	}

	protocolFeeRecipient = solana.PublicKeyFromBytes(make([]byte, 32))
	if len(globalConfig.ProtocolFeeRecipients) > 0 {
		protocolFeeRecipient = globalConfig.ProtocolFeeRecipients[0]
	}
	protocolFeeRecipientATA, _, err = solana.FindAssociatedTokenAddress(
		protocolFeeRecipient,
		pool.QuoteMint,
	)
	return
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
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		blockhash, err := dex.client.GetRecentBlockhash(ctx)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(dex.wallet.PublicKey))
		if err != nil {
			return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
		}
		if err = dex.wallet.SignTransaction(tx); err != nil {
			return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
		}
		sig, err := dex.client.SendTransaction(ctx, tx)
		if err != nil {
			if strings.Contains(err.Error(), "BlockhashNotFound") {
				time.Sleep(500 * time.Millisecond)
				lastErr = err
				continue
			}
			return solana.Signature{}, err
		}
		if err = dex.client.WaitForTransactionConfirmation(ctx, sig, rpc.CommitmentConfirmed); err != nil {
			return sig, fmt.Errorf("transaction failed: %w", err)
		}
		return sig, nil
	}
	return solana.Signature{}, fmt.Errorf("all attempts failed: %w", lastErr)
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

	userBaseATA, userQuoteATA, protocolFeeRecipientATA, protocolFeeRecipient, createBaseIx, createQuoteIx, err := dex.prepareTokenAccounts(ctx, pool)
	if err != nil {
		return err
	}

	priorityInstructions, err := dex.preparePriorityInstructions(computeUnits, priorityFeeSol)
	if err != nil {
		return err
	}

	var instructions []solana.Instruction
	instructions = append(instructions, priorityInstructions...)
	instructions = append(instructions, createBaseIx, createQuoteIx)

	// Получаем точность токенов.
	baseDecimals := dex.getTokenDecimals(ctx, pool.BaseMint, 6)   // Точность для токена (покупаемого)
	quoteDecimals := dex.getTokenDecimals(ctx, pool.QuoteMint, 9) // Для WSOL обычно 9

	dex.logger.Debug("Token decimals",
		zap.Uint8("base_decimals", baseDecimals),
		zap.Uint8("quote_decimals", quoteDecimals),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	var swapIx solana.Instruction
	if isBuy {
		// При покупке мы хотим получить определённое количество токена (baseAmountOut),
		// и готовы заплатить максимум amount (WSOL).
		outputAmount, price := dex.poolManager.CalculateSwapQuote(pool, amount, true)
		minOut := uint64(float64(outputAmount) * (1.0 - slippagePercent/100.0))
		dex.logger.Debug("Buy swap calculation",
			zap.Uint64("input_amount", amount),
			zap.Uint64("expected_output", outputAmount),
			zap.Uint64("min_out_amount", minOut),
			zap.Float64("price", price))
		swapIx = createBuyInstruction(
			pool.Address,
			dex.wallet.PublicKey,
			dex.config.GlobalConfig,
			pool.BaseMint,  // Токен, который покупаем
			pool.QuoteMint, // WSOL
			userBaseATA,
			userQuoteATA,
			pool.PoolBaseTokenAccount,
			pool.PoolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientATA,
			TokenProgramID,
			TokenProgramID,
			dex.config.EventAuthority,
			dex.config.ProgramID,
			outputAmount,
			amount,
		)
	} else {
		// Продажа: продаём токен (base) за WSOL.
		expectedOutput, price := dex.poolManager.CalculateSwapQuote(pool, amount, false)
		minOut := uint64(float64(expectedOutput) * (1.0 - slippagePercent/100.0))
		dex.logger.Debug("Sell swap calculation",
			zap.Uint64("input_amount", amount),
			zap.Uint64("expected_output", expectedOutput),
			zap.Uint64("min_out_amount", minOut),
			zap.Float64("price", price))
		swapIx = createSellInstruction(
			pool.Address,
			dex.wallet.PublicKey,
			dex.config.GlobalConfig,
			pool.BaseMint,
			pool.QuoteMint,
			userBaseATA,
			userQuoteATA,
			pool.PoolBaseTokenAccount,
			pool.PoolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientATA,
			TokenProgramID,
			TokenProgramID,
			dex.config.EventAuthority,
			dex.config.ProgramID,
			amount,
			minOut,
		)
	}

	instructions = append(instructions, swapIx)
	sig, err := dex.buildAndSubmitTransaction(ctx, instructions)
	if err != nil {
		// В случае ошибки для микротранзакций (например, amount <= 100_000) можно добавить логику повторной попытки.
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
		solDecimals := uint8(9)
		tokenDecimals := dex.getTokenDecimals(ctx, pool.BaseMint, 6)
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
	info, err := dex.client.GetAccountInfo(ctx, mint)
	if err != nil {
		return 0, fmt.Errorf("failed to get mint info: %w", err)
	}
	if info == nil || info.Value == nil {
		return 0, fmt.Errorf("mint account not found")
	}
	data := info.Value.Data.GetBinary()
	if len(data) < 45 {
		return 0, fmt.Errorf("mint account data too short")
	}
	return data[44], nil
}
