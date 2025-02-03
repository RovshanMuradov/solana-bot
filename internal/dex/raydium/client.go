// internal/dex/raydium/client.go
package raydium

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// NewRaydiumClient создает новый экземпляр клиента
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
		api:         NewAPIService(logger),
	}, nil
}

// GetPool получает информацию о пуле для пары токенов
func (c *Client) GetPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*Pool, error) {
	c.logger.Debug("searching pool",
		zap.String("baseMint", baseMint.String()),
		zap.String("quoteMint", quoteMint.String()))

	// Получаем пул через API для базового токена
	basePool, err := c.api.GetPoolByToken(ctx, baseMint)
	if err == nil && basePool != nil &&
		(basePool.BaseMint.Equals(quoteMint) || basePool.QuoteMint.Equals(quoteMint)) {

		if err := c.enrichAndValidatePool(ctx, basePool, baseMint, quoteMint); err == nil {
			return basePool, nil
		}
	}

	// Если не нашли через базовый токен, пробуем через котируемый
	if !quoteMint.Equals(baseMint) {
		quotePool, err := c.api.GetPoolByToken(ctx, quoteMint)
		if err == nil && quotePool != nil &&
			(quotePool.BaseMint.Equals(baseMint) || quotePool.QuoteMint.Equals(baseMint)) {

			if err := c.enrichAndValidatePool(ctx, quotePool, baseMint, quoteMint); err == nil {
				return quotePool, nil
			}
		}
	}

	return nil, fmt.Errorf("no viable pools found for tokens %s and %s",
		baseMint.String(), quoteMint.String())
}

// enrichAndValidatePool обогащает пул данными и проверяет его валидность
func (c *Client) enrichAndValidatePool(ctx context.Context, pool *Pool, baseMint, quoteMint solana.PublicKey) error {
	// Получаем данные пула из блокчейна
	account, err := c.client.GetAccountInfo(ctx, pool.ID)
	if err != nil {
		return fmt.Errorf("failed to get pool account: %w", err)
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
		return fmt.Errorf("failed to derive authority: %w", err)
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

	// Обновляем состояние
	pool.State = PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(data[64:72]),
		QuoteReserve: binary.LittleEndian.Uint64(data[72:80]),
		Status:       data[88],
	}

	// Проверяем соответствие токенов
	if !((pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
		(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint))) {
		return fmt.Errorf("pool tokens do not match requested tokens")
	}

	return c.checkPoolLiquidity(ctx, pool)
}

// checkPoolLiquidity проверяет достаточность ликвидности в пуле
func (c *Client) checkPoolLiquidity(ctx context.Context, pool *Pool) error {
	if pool.State.Status != PoolStatusActive {
		return fmt.Errorf("pool is not active")
	}

	// Проверяем базовую ликвидность
	if pool.State.BaseReserve == 0 || pool.State.QuoteReserve == 0 {
		return fmt.Errorf("pool has no liquidity")
	}

	// Добавляем расширенную проверку ликвидности
	if err := CheckLiquiditySufficiency(pool, 0); err != nil {
		return fmt.Errorf("liquidity check failed: %w", err)
	}
	// Проверяем критические метрики пула
	baseToQuoteRatio := float64(pool.State.BaseReserve) / float64(pool.State.QuoteReserve)
	const maxRatioDeviation = 10.0

	if baseToQuoteRatio > maxRatioDeviation || baseToQuoteRatio < 1/maxRatioDeviation {
		return fmt.Errorf("pool reserves too imbalanced: ratio=%.2f", baseToQuoteRatio)
	}

	return nil
}

// ensureTokenAccounts проверяет и создает ATA при необходимости
func (c *Client) ensureTokenAccounts(ctx context.Context, sourceToken, targetToken solana.PublicKey) (*TokenAccounts, error) {
	accounts := &TokenAccounts{}
	var created bool

	// Получаем ATA для source токена
	sourceATA, _, err := solana.FindAssociatedTokenAddress(
		c.GetPublicKey(),
		sourceToken,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find source ATA: %w", err)
	}
	accounts.SourceATA = sourceATA

	// Получаем ATA для target токена
	targetATA, _, err := solana.FindAssociatedTokenAddress(
		c.GetPublicKey(),
		targetToken,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find target ATA: %w", err)
	}
	accounts.DestinationATA = targetATA

	// Проверяем существование ATA
	if exists, err := c.checkTokenAccount(ctx, sourceATA); err != nil {
		return nil, err
	} else if !exists {
		if err := c.createTokenAccount(ctx, sourceToken); err != nil {
			return nil, err
		}
		created = true
	}

	if exists, err := c.checkTokenAccount(ctx, targetATA); err != nil {
		return nil, err
	} else if !exists {
		if err := c.createTokenAccount(ctx, targetToken); err != nil {
			return nil, err
		}
		created = true
	}

	accounts.Created = created
	return accounts, nil
}

// Вспомогательные методы
func (c *Client) checkTokenAccount(ctx context.Context, account solana.PublicKey) (bool, error) {
	acc, err := c.client.GetAccountInfo(ctx, account)
	if err != nil {
		return false, fmt.Errorf("failed to get token account: %w", err)
	}
	return acc != nil && acc.Value != nil, nil
}

func (c *Client) createTokenAccount(ctx context.Context, mint solana.PublicKey) error {
	// Создаем инструкцию для создания ассоциированного токен аккаунта
	instruction := token.NewInitializeAccount3InstructionBuilder().
		SetAccount(c.GetPublicKey()).
		SetMintAccount(mint).
		SetOwner(c.GetPublicKey()).
		Build()

	// Получаем последний блокхэш
	recentBlockHash, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем транзакцию
	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recentBlockHash,
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Отправляем транзакцию
	_, err = c.client.SendTransaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to create token account: %w", err)
	}

	return nil
}

func (c *Client) GetPublicKey() solana.PublicKey {
	return c.privateKey.PublicKey()
}

func (c *Client) GetBaseClient() blockchain.Client {
	return c.client
}

// validateAndPrepareSwap объединяет подготовку и валидацию свапа
func (c *Client) validateAndPrepareSwap(ctx context.Context, params *SwapParams) error {
	if params == nil {
		return fmt.Errorf("swap params cannot be nil")
	}

	c.logger.Debug("preparing swap parameters",
		zap.String("pool_id", params.Pool.ID.String()),
		zap.Uint64("amount_in", params.AmountIn))

	// Проверяем валидность пула через API
	if err := c.api.ValidatePool(ctx, params.Pool); err != nil {
		return fmt.Errorf("pool validation failed: %w", err)
	}

	// Проверяем и подготавливаем токен-аккаунты
	accounts, err := c.ensureTokenAccounts(ctx, params.Pool.BaseMint, params.Pool.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to ensure token accounts: %w", err)
	}
	params.SourceTokenAccount = accounts.SourceATA
	params.DestinationTokenAccount = accounts.DestinationATA

	// Проверяем баланс кошелька
	balance, err := c.client.GetBalance(ctx, params.UserWallet, c.commitment)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %w", err)
	}

	// Расчет необходимого баланса (сумма свапа + комиссия + приоритетная комиссия)
	requiredBalance := params.AmountIn + params.PriorityFeeLamports + 5000 // 5000 lamports для комиссии сети
	if balance < requiredBalance {
		return fmt.Errorf("insufficient balance: required %d, got %d", requiredBalance, balance)
	}

	// Проверяем влияние на цену
	impact := GetPriceImpact(params.Pool, params.AmountIn)
	if impact > float64(params.SlippageBps)/100 {
		return fmt.Errorf("price impact too high: %.2f%% > %.2f%%",
			impact, float64(params.SlippageBps)/100)
	}

	c.logger.Debug("swap preparation completed",
		zap.String("source_ata", accounts.SourceATA.String()),
		zap.String("destination_ata", accounts.DestinationATA.String()),
		zap.Float64("price_impact", impact))

	return nil
}

// DetermineSwapDirection определяет направление свапа на основе пары токенов
func (c *Client) DetermineSwapDirection(pool *Pool, sourceToken, targetToken solana.PublicKey) (SwapDirection, error) {
	if pool == nil {
		return SwapDirectionIn, fmt.Errorf("pool cannot be nil")
	}

	c.logger.Debug("determining swap direction",
		zap.String("source_token", sourceToken.String()),
		zap.String("target_token", targetToken.String()),
		zap.String("pool_base", pool.BaseMint.String()),
		zap.String("pool_quote", pool.QuoteMint.String()))

	// Проверяем соответствие токенов пулу
	isSourceBase := sourceToken.Equals(pool.BaseMint)
	isSourceQuote := sourceToken.Equals(pool.QuoteMint)
	isTargetBase := targetToken.Equals(pool.BaseMint)
	isTargetQuote := targetToken.Equals(pool.QuoteMint)

	// Определяем направление
	switch {
	case isSourceBase && isTargetQuote:
		c.logger.Debug("swap direction: base to quote (IN)")
		return SwapDirectionIn, nil
	case isSourceQuote && isTargetBase:
		c.logger.Debug("swap direction: quote to base (OUT)")
		return SwapDirectionOut, nil
	default:
		return SwapDirectionIn, fmt.Errorf("invalid token pair: source=%s, target=%s",
			sourceToken.String(), targetToken.String())
	}
}

// ValidateTokenPair проверяет валидность пары токенов для свапа
func (c *Client) ValidateTokenPair(pool *Pool, sourceToken, targetToken solana.PublicKey) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	c.logger.Debug("validating token pair",
		zap.String("source_token", sourceToken.String()),
		zap.String("target_token", targetToken.String()))

	// Проверяем существование токенов
	if sourceToken.IsZero() || targetToken.IsZero() {
		return fmt.Errorf("invalid token addresses: source=%s, target=%s",
			sourceToken.String(), targetToken.String())
	}

	// Проверяем decimals
	if pool.BaseDecimals == 0 || pool.QuoteDecimals == 0 {
		return fmt.Errorf("invalid token decimals: base=%d, quote=%d",
			pool.BaseDecimals, pool.QuoteDecimals)
	}

	// Проверяем поддержку токенов в пуле
	isSourceSupported := sourceToken.Equals(pool.BaseMint) || sourceToken.Equals(pool.QuoteMint)
	isTargetSupported := targetToken.Equals(pool.BaseMint) || targetToken.Equals(pool.QuoteMint)

	if !isSourceSupported {
		return fmt.Errorf("source token %s not supported in pool %s",
			sourceToken.String(), pool.ID.String())
	}
	if !isTargetSupported {
		return fmt.Errorf("target token %s not supported in pool %s",
			targetToken.String(), pool.ID.String())
	}

	// Проверяем возможность прямого свапа
	if sourceToken.Equals(targetToken) {
		return fmt.Errorf("source and target tokens are the same: %s",
			sourceToken.String())
	}

	// Проверяем состояние пула для этой пары
	if pool.State.Status != PoolStatusActive {
		return fmt.Errorf("pool %s is not active for token pair %s-%s",
			pool.ID.String(), sourceToken.String(), targetToken.String())
	}

	c.logger.Debug("token pair validation successful",
		zap.String("pool_id", pool.ID.String()),
		zap.String("source_token", sourceToken.String()),
		zap.String("target_token", targetToken.String()))

	return nil
}

// MonitorPoolState отслеживает состояние пула
func (c *Client) MonitorPoolState(ctx context.Context, pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	ticker := time.NewTicker(500 * time.Millisecond) // Частота проверки
	defer ticker.Stop()

	var lastState PoolState
	var significantChangesCount int

	c.logger.Info("starting pool monitoring",
		zap.String("pool_id", pool.ID.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Получаем актуальное состояние пула
			account, err := c.client.GetAccountInfo(ctx, pool.ID)
			if err != nil {
				c.logger.Error("failed to get pool account",
					zap.Error(err),
					zap.String("pool_id", pool.ID.String()))
				continue
			}

			if account == nil || account.Value == nil {
				c.logger.Warn("pool account not found",
					zap.String("pool_id", pool.ID.String()))
				continue
			}

			// Обновляем состояние пула
			currentState := PoolState{
				BaseReserve:  binary.LittleEndian.Uint64(account.Value.Data.GetBinary()[64:72]),
				QuoteReserve: binary.LittleEndian.Uint64(account.Value.Data.GetBinary()[72:80]),
				Status:       account.Value.Data.GetBinary()[88],
			}

			// Проверяем изменение статуса
			if currentState.Status != lastState.Status {
				c.logger.Warn("pool status changed",
					zap.Uint8("old_status", lastState.Status),
					zap.Uint8("new_status", currentState.Status))

				if currentState.Status != PoolStatusActive {
					return fmt.Errorf("pool became inactive: status=%d", currentState.Status)
				}
			}

			// Проверяем изменения в ликвидности
			baseChange := float64(currentState.BaseReserve-lastState.BaseReserve) / float64(lastState.BaseReserve)
			quoteChange := float64(currentState.QuoteReserve-lastState.QuoteReserve) / float64(lastState.QuoteReserve)

			const significantChangeThreshold = 0.02 // 2%
			if math.Abs(baseChange) > significantChangeThreshold || math.Abs(quoteChange) > significantChangeThreshold {
				significantChangesCount++
				c.logger.Info("significant liquidity change detected",
					zap.Float64("base_change_percent", baseChange*100),
					zap.Float64("quote_change_percent", quoteChange*100),
					zap.Uint64("base_reserve", currentState.BaseReserve),
					zap.Uint64("quote_reserve", currentState.QuoteReserve))

				// Если слишком много существенных изменений за короткий период
				if significantChangesCount > 5 {
					c.logger.Warn("high volatility detected in pool",
						zap.String("pool_id", pool.ID.String()),
						zap.Int("changes_count", significantChangesCount))
				}
			} else {
				significantChangesCount = 0 // Сбрасываем счетчик при стабилизации
			}

			lastState = currentState
		}
	}
}

// RetrySwap выполняет свап с механизмом повторных попыток
func (c *Client) RetrySwap(ctx context.Context, params *SwapParams) (*SwapResult, error) {
	if params == nil {
		return nil, fmt.Errorf("swap params cannot be nil")
	}

	result := &SwapResult{
		AmountIn:   params.AmountIn,
		RetryCount: 0,
	}

	// Конфигурация retry
	const (
		maxRetries = 3
		baseDelay  = 1 * time.Second
	)

	var lastError error
	startTime := time.Now()

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Проверяем контекст перед каждой попыткой
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("context cancelled during retry: %w", err)
		}

		// Логируем попытку
		if attempt > 0 {
			c.logger.Info("retrying swap",
				zap.Int("attempt", attempt),
				zap.Duration("elapsed", time.Since(startTime)),
				zap.Error(lastError))
		}

		// Выполняем свап
		signature, err := c.Swap(ctx, params)
		if err == nil {
			result.Signature = signature
			result.ExecutionTime = time.Since(startTime)
			result.RetryCount = attempt

			// Ждем подтверждения, если требуется
			if params.WaitConfirmation {
				if err := c.WaitForConfirmation(ctx, signature); err != nil {
					lastError = fmt.Errorf("confirmation failed: %w", err)
					continue
				}
				result.Confirmed = true
			}

			// Получаем информацию о транзакции
			tx, err := c.client.GetTransaction(ctx, signature)
			if err == nil && tx != nil {
				result.BlockTime = time.Unix(int64(*tx.BlockTime), 0)
				// Можно добавить расчет фактического AmountOut и FeesPaid
			}

			return result, nil
		}

		lastError = err

		// Определяем, стоит ли повторять попытку
		if !c.shouldRetry(err) {
			c.logger.Error("non-retriable error occurred",
				zap.Error(err),
				zap.Int("attempt", attempt))
			break
		}

		// Экспоненциальная задержка между попытками
		if attempt < maxRetries {
			delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
	}

	result.Error = lastError
	return result, fmt.Errorf("max retries exceeded: %w", lastError)
}

// shouldRetry определяет, следует ли повторить попытку на основе типа ошибки
func (c *Client) shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Список ошибок, при которых стоит повторить попытку
	retriableErrors := []string{
		"timeout",
		"connection refused",
		"no recent blockhash",
		"blockhash not found",
		"insufficient funds for transaction",
	}

	errStr := strings.ToLower(err.Error())
	for _, retriableErr := range retriableErrors {
		if strings.Contains(errStr, retriableErr) {
			return true
		}
	}

	return false
}

// ValidateSwapResult проверяет результат свапа
func (c *Client) ValidateSwapResult(ctx context.Context, result *SwapResult, params *SwapParams) error {
	if result == nil || params == nil {
		return fmt.Errorf("result and params cannot be nil")
	}

	c.logger.Debug("validating swap result",
		zap.String("signature", result.Signature.String()),
		zap.Uint64("amount_in", params.AmountIn),
		zap.Uint64("min_amount_out", params.MinAmountOut))

	// Проверяем успешность транзакции
	status, err := c.client.GetSignatureStatuses(ctx, result.Signature)
	if err != nil {
		return fmt.Errorf("failed to get transaction status: %w", err)
	}

	if status == nil || len(status.Value) == 0 || status.Value[0] == nil {
		return fmt.Errorf("transaction status not found")
	}

	if status.Value[0].Err != nil {
		return fmt.Errorf("transaction failed: %v", status.Value[0].Err)
	}

	// Получаем балансы после свапа
	sourceBalance, err := c.getTokenBalance(ctx, params.SourceTokenAccount)
	if err != nil {
		return fmt.Errorf("failed to get source token balance: %w", err)
	}

	destinationBalance, err := c.getTokenBalance(ctx, params.DestinationTokenAccount)
	if err != nil {
		return fmt.Errorf("failed to get destination token balance: %w", err)
	}

	// Проверяем соответствие слиппажу
	if result.AmountOut < params.MinAmountOut {
		return fmt.Errorf("received amount %d is less than minimum expected %d",
			result.AmountOut, params.MinAmountOut)
	}

	// Проверяем изменение баланса source токена
	expectedSourceBalance := sourceBalance + params.AmountIn
	if math.Abs(float64(expectedSourceBalance-sourceBalance)) > float64(params.AmountIn)*0.001 { // допуск 0.1%
		c.logger.Warn("unexpected source balance change",
			zap.Uint64("expected", expectedSourceBalance),
			zap.Uint64("actual", sourceBalance))
	}

	// Проверяем изменение баланса destination токена
	if destinationBalance < result.AmountOut {
		c.logger.Warn("unexpected destination balance",
			zap.Uint64("expected_min", result.AmountOut),
			zap.Uint64("actual", destinationBalance))
	}

	// Рассчитываем реальный слиппаж
	expectedPrice := float64(params.AmountIn) / float64(params.MinAmountOut)
	actualPrice := float64(params.AmountIn) / float64(result.AmountOut)
	actualSlippage := math.Abs((actualPrice-expectedPrice)/expectedPrice) * 100

	// Проверяем, не превышен ли максимальный слиппаж
	if actualSlippage > float64(params.SlippageBps)/100 {
		c.logger.Warn("high slippage detected",
			zap.Float64("expected_price", expectedPrice),
			zap.Float64("actual_price", actualPrice),
			zap.Float64("slippage_percent", actualSlippage))
	}

	// Проверяем время исполнения
	if result.ExecutionTime > 30*time.Second {
		c.logger.Warn("slow transaction execution",
			zap.Duration("execution_time", result.ExecutionTime))
	}

	// Проверяем комиссии
	if result.FeesPaid > params.PriorityFeeLamports*2 {
		c.logger.Warn("high transaction fees",
			zap.Uint64("expected_fee", params.PriorityFeeLamports),
			zap.Uint64("actual_fee", result.FeesPaid))
	}

	// Формируем детальный отчет о свапе
	c.logger.Info("swap validation completed",
		zap.String("signature", result.Signature.String()),
		zap.Uint64("amount_in", params.AmountIn),
		zap.Uint64("amount_out", result.AmountOut),
		zap.Uint64("min_amount_out", params.MinAmountOut),
		zap.Float64("slippage_percent", actualSlippage),
		zap.Uint64("fees_paid", result.FeesPaid),
		zap.Duration("execution_time", result.ExecutionTime),
		zap.Time("block_time", result.BlockTime),
		zap.Int("retry_count", result.RetryCount))

	return nil
}

// getTokenBalance получает баланс токена для аккаунта
func (c *Client) getTokenBalance(ctx context.Context, account solana.PublicKey) (uint64, error) {
	resp, err := c.client.GetTokenAccountBalance(ctx, account, c.commitment)
	if err != nil {
		return 0, fmt.Errorf("failed to get token balance: %w", err)
	}

	if resp == nil || resp.Value == nil {
		return 0, fmt.Errorf("empty token balance response")
	}

	balance, err := strconv.ParseUint(resp.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token balance: %w", err)
	}

	return balance, nil
}
