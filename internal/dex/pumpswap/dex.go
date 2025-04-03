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
//
// Функция инициализирует объект DEX, который предоставляет интерфейс для взаимодействия
// с децентрализованной биржей PumpSwap. При создании проверяется валидность всех
// обязательных параметров и применяются конфигурационные настройки.
//
// Параметры:
//   - client: клиент Solana для взаимодействия с блокчейном
//   - w: кошелёк для подписания транзакций
//   - logger: логгер для записи информации о работе DEX
//   - config: конфигурация DEX с настройками
//   - poolManager: интерфейс для работы с пулами ликвидности
//   - monitorInterval: интервал мониторинга (если пустая строка, используется значение из config)
//
// Возвращает:
//   - *DEX: инициализированный экземпляр DEX
//   - error: ошибка, если не удалось создать экземпляр DEX
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
//
// Функция объединяет приоритетные инструкции (для установки комиссии и лимита вычислительных единиц)
// с основными инструкциями для выполнения свапа. Метод является частью внутренней реализации
// операции обмена токенов.
//
// Параметры:
//   - pool: информация о пуле ликвидности, в котором будет осуществляться обмен
//   - accounts: подготовленные токен-аккаунты для операции свапа
//   - params: параметры операции свапа (тип операции, количество, проскальзывание и др.)
//   - amounts: рассчитанные суммы для операции обмена (базовая и котировочная)
//
// Возвращает:
//   - []solana.Instruction: массив инструкций для транзакции свапа
//   - error: ошибка при подготовке инструкций
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
//
// Это удобная обертка вокруг метода ExecuteSwap, которая настраивает параметры
// для выполнения операции продажи токена. Метод автоматически устанавливает флаг
// IsBuy в false и передает все необходимые параметры в ExecuteSwap.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - tokenAmount: количество токенов для продажи (в минимальных единицах)
//   - slippagePercent: допустимое проскальзывание в процентах (0-100)
//   - priorityFeeSol: приоритетная комиссия в SOL (строковое представление)
//   - computeUnits: лимит вычислительных единиц для транзакции
//
// Возвращает:
//   - error: ошибка при выполнении операции продажи
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
//
// Метод находит активный пул ликвидности для указанного токена и вычисляет
// текущую цену на основе соотношения резервов базового токена (tokenMint) и
// котировочного токена (WSOL). Цена корректируется с учетом разного количества
// десятичных знаков у токенов.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - tokenMint: адрес минта токена в формате Base58
//
// Возвращает:
//   - float64: цена токена относительно SOL
//   - error: ошибка при вычислении цены токена
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
//
// Метод определяет баланс указанного токена в ассоциированном токен-аккаунте (ATA)
// пользователя. Адрес ATA вычисляется на основе публичного ключа кошелька и
// минта токена. Метод проверяет соответствие указанного токенового минта
// ожидаемому базовому минту DEX.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - tokenMint: адрес минта токена в формате Base58
//
// Возвращает:
//   - uint64: баланс токена в минимальных единицах
//   - error: ошибка при получении баланса токена
func (d *DEX) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// Проверяем соответствие tokenMint
	mint, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}

	effBase, _ := d.effectiveMints()
	if !mint.Equals(effBase) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s", effBase.String(), mint.String())
	}

	// Находим ATA адрес для токена
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, mint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Получаем баланс токена
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, rpc.CommitmentConfirmed)
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
