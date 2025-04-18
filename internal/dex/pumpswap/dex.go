// =============================
// File: internal/dex/pumpswap/dex.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
	"math"
	"math/big"
	"strconv"
	"time"
)

// NewDEX создаёт новый экземпляр DEX для PumpSwap.
func NewDEX(
	client *solbc.Client,
	w *wallet.Wallet,
	logger *zap.Logger,
	config *Config,
	poolManager PoolManagerInterface, // теперь передаём интерфейс
	monitorInterval string,
) (*DEX, error) {
	if client == nil || w == nil || logger == nil || config == nil || poolManager == nil {
		return nil, fmt.Errorf("client, wallet, logger, poolManager и config не могут быть nil")
	}
	if monitorInterval != "" {
		config.MonitorInterval = monitorInterval
	}
	return &DEX{
		client:      client,
		wallet:      w,
		logger:      logger,
		config:      config,
		poolManager: poolManager,
	}, nil
}

// ExecuteSwap выполняет операцию обмена на DEX.
func (d *DEX) ExecuteSwap(ctx context.Context, params SwapParams) error {
	pool, _, err := d.findAndValidatePool(ctx)
	if err != nil {
		return err
	}

	accounts, err := d.prepareTokenAccounts(ctx, pool)
	if err != nil {
		return err
	}

	// Вычисляем параметры для свапа
	amounts := d.calculateSwapAmounts(pool, params.IsBuy, params.Amount, params.SlippagePercent)

	// Подготавливаем инструкции для транзакции
	instructions, err := d.prepareSwapInstructions(pool, accounts, params, amounts)
	if err != nil {
		return err
	}

	sig, err := d.buildAndSubmitTransaction(ctx, instructions)
	if err != nil {
		return d.handleSwapError(err, params)
	}

	d.logger.Info("Swap executed successfully",
		zap.String("signature", sig.String()),
		zap.Bool("is_buy", params.IsBuy),
		zap.Uint64("amount", params.Amount))
	return nil
}

// prepareSwapInstructions подготавливает инструкции для выполнения операции свапа.
func (d *DEX) prepareSwapInstructions(pool *PoolInfo, accounts *PreparedTokenAccounts,
	params SwapParams, amounts *SwapAmounts) ([]solana.Instruction, error) {

	priorityInstructions, err := d.preparePriorityInstructions(params.ComputeUnits, params.PriorityFeeSol)
	if err != nil {
		return nil, err
	}

	return d.buildSwapTransaction(pool, accounts, params.IsBuy, amounts.BaseAmount,
		amounts.QuoteAmount, priorityInstructions), nil
}

// ExecuteSell выполняет операцию продажи токена за WSOL.
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

// GetTokenBalance получает баланс токена в кошельке пользователя.
func (d *DEX) GetTokenBalance(ctx context.Context, commitment ...rpc.CommitmentType) (uint64, error) {
	// Находим ATA адрес для токена
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.QuoteMint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Шаг 2: Определение уровня подтверждения (commitment level)
	// По умолчанию используем Processed - самый быстрый уровень
	// Можно переопределить через вариативный параметр
	commitmentLevel := rpc.CommitmentProcessed
	if len(commitment) > 0 {
		commitmentLevel = commitment[0]
	}

	// Получаем баланс токена
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)

	// Шаг 4: При неудаче с Processed, пробуем Confirmed
	if err != nil && commitmentLevel == rpc.CommitmentProcessed {
		d.logger.Debug("Failed to get balance with Processed commitment, trying Confirmed",
			zap.String("token_mint", d.config.QuoteMint.String()),
			zap.String("user_ata", userATA.String()),
			zap.Error(err))

		// Повторный запрос с более надежным уровнем подтверждения
		result, err = d.client.GetTokenAccountBalance(ctx, userATA, rpc.CommitmentConfirmed)
	}

	// Проверяем ошибку после возможной повторной попытки
	if err != nil {
		return 0, fmt.Errorf("failed to get token account balance: %w", err)
	}

	if result == nil || result.Value.Amount == "" {
		return 0, fmt.Errorf("no token balance found")
	}

	// Парсим результат в uint64
	balance, err := strconv.ParseUint(result.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token balance: %w", err)
	}

	return balance, nil
}

// SellPercentTokens продает указанный процент от доступного баланса токенов.
// Метод получает баланс токена, рассчитывает сумму для продажи в соответствии
// с указанным процентом и выполняет операцию продажи.
//
// Параметры:
//   - ctx: контекст выполнения
//   - percentToSell: процент от общего баланса для продажи (от 0.0 до 100.0)
//   - slippagePercent: максимально допустимое проскальзывание цены в процентах
//   - priorityFeeSol: приоритетная комиссия в SOL (строковое представление)
//   - computeUnits: количество вычислительных единиц для транзакции
//
// Возвращает:
//   - error: ошибку, если операция не удалась, или nil при успешном выполнении
func (d *DEX) SellPercentTokens(ctx context.Context, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	// Проверка валидности параметра percentToSell
	if percentToSell <= 0 || percentToSell > 100 {
		return fmt.Errorf("percentToSell должен быть в пределах от 0 до 100, получено: %f", percentToSell)
	}

	// Получаем текущий баланс токена
	tokenBalance, err := d.GetTokenBalance(ctx)
	if err != nil {
		return fmt.Errorf("не удалось получить баланс токена: %w", err)
	}

	// Проверяем, есть ли токены для продажи
	if tokenBalance == 0 {
		return fmt.Errorf("нет токенов для продажи")
	}

	// Рассчитываем количество токенов для продажи
	amountToSell := uint64(float64(tokenBalance) * percentToSell / 100.0)
	
	// Убедимся, что продаём хотя бы 1 токен, если есть баланс
	if amountToSell == 0 && tokenBalance > 0 {
		amountToSell = 1
	}

	d.logger.Info("Продажа токенов",
		zap.Uint64("current_balance", tokenBalance),
		zap.Float64("percent_to_sell", percentToSell),
		zap.Uint64("amount_to_sell", amountToSell),
		zap.Float64("slippage_percent", slippagePercent))

	// Выполняем продажу указанного количества токенов
	return d.ExecuteSell(ctx, amountToSell, slippagePercent, priorityFeeSol, computeUnits)
}
