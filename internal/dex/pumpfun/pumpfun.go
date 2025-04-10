// =============================
// File: internal/dex/pumpfun/pumpfun.go
// =============================
package pumpfun

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX представляет собой имплементацию интерфейса для взаимодействия с Pump.fun.
type DEX struct {
	client          *solbc.Client
	wallet          *wallet.Wallet
	logger          *zap.Logger
	config          *Config
	priorityManager *types.PriorityManager // TODO: не используемый параметр. Рассмотреть удаление
}

// NewDEX создает новый экземпляр DEX для работы с Pump.fun.
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, _ string) (*DEX, error) {
	// Проверяем, что адрес контракта Pump.fun указан
	if config.ContractAddress.IsZero() {
		return nil, fmt.Errorf("pump.fun contract address is required")
	}
	// Проверяем, что адрес минта токена указан
	if config.Mint.IsZero() {
		return nil, fmt.Errorf("token mint address is required")
	}

	// Логируем информацию о создании DEX
	logger.Info("Creating PumpFun DEX",
		zap.String("contract", config.ContractAddress.String()),
		zap.String("token_mint", config.Mint.String()))

	// Создаем экземпляр DEX с базовыми параметрами
	dex := &DEX{
		client:          client,
		wallet:          w,
		logger:          logger.Named("pumpfun"),
		config:          config,
		priorityManager: types.NewPriorityManager(logger.Named("priority")),
	}

	// Создаем контекст с таймаутом для получения данных о глобальном аккаунте
	fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем информацию о глобальном аккаунте для определения получателя комиссий
	globalAccount, err := FetchGlobalAccount(fetchCtx, client, config.Global)
	if err != nil {
		logger.Warn("Failed to fetch global account data, using default fee recipient",
			zap.Error(err))
	} else if globalAccount != nil {
		// Обновляем адрес получателя комиссий из глобального аккаунта
		config.FeeRecipient = globalAccount.FeeRecipient
		logger.Info("Updated fee recipient", zap.String("address", config.FeeRecipient.String()))
	}

	return dex, nil
}

// ExecuteSnipe выполняет операцию покупки токена на Pump.fun с точным количеством SOL.
func (d *DEX) ExecuteSnipe(ctx context.Context, amountSol float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	// Логируем информацию о начале операции
	d.logger.Info("Starting Pump.fun exact-sol buy operation",
		zap.Float64("amount_sol", amountSol),
		zap.Float64("slippage_percent", slippagePercent),
		zap.String("priority_fee_sol", priorityFeeSol),
		zap.Uint32("compute_units", computeUnits))

	// Создаем контекст с таймаутом для выполнения транзакции
	opCtx, cancel := d.prepareTransactionContext(ctx, 45*time.Second)
	defer cancel()

	// Конвертируем SOL в ламппорты (1 SOL = 10^9 ламппортов)
	solAmountLamports := uint64(amountSol * 1_000_000_000)

	// Логируем точное количество SOL для покупки
	d.logger.Info("Using exact SOL amount",
		zap.Uint64("sol_amount_lamports", solAmountLamports),
		zap.String("sol_amount", fmt.Sprintf("%.9f SOL", float64(solAmountLamports)/1_000_000_000)))

	// Подготавливаем инструкции для транзакции покупки
	instructions, err := d.prepareBuyTransaction(opCtx, solAmountLamports, priorityFeeSol, computeUnits)
	// TODO: пересмотреть логику solAmountLamports, priorityFeeSol, computeUnits. Данные должны брать из config.json and tasks.csv

	if err != nil {
		return err
	}

	// Отправляем и ожидаем подтверждения транзакции
	_, err = d.sendAndConfirmTransaction(opCtx, instructions)
	return err
}

// ExecuteSell выполняет операцию продажи токена на Pump.fun.
func (d *DEX) ExecuteSell(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	// Логируем информацию о начале операции продажи
	d.logger.Info("Starting Pump.fun sell operation",
		zap.Uint64("token_amount", tokenAmount),
		zap.Float64("slippage_percent", slippagePercent),
		zap.String("priority_fee_sol", priorityFeeSol),
		zap.Uint32("compute_units", computeUnits))

	// Создаем контекст с таймаутом для выполнения транзакции
	opCtx, cancel := d.prepareTransactionContext(ctx, 45*time.Second)
	defer cancel()

	// Подготавливаем инструкции для транзакции продажи
	instructions, err := d.prepareSellTransaction(opCtx, tokenAmount, slippagePercent, priorityFeeSol, computeUnits)
	// TODO: тоже пересмотреть логику

	if err != nil {
		return err
	}

	// Отправляем и ожидаем подтверждения транзакции
	_, err = d.sendAndConfirmTransaction(opCtx, instructions)
	if err != nil {
		// Обрабатываем специфические ошибки продажи (например, если токен перемещен на Raydium)
		return d.handleSellError(err)
	}

	return nil
}

// SellPercentTokens продает указанный процент от доступного баланса токенов.
func (d *DEX) SellPercentTokens(ctx context.Context, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	// Проверяем, что процент находится в допустимом диапазоне
	if percentToSell <= 0 || percentToSell > 100 {
		return fmt.Errorf("percent to sell must be between 0 and 100")
	}

	// Создаем контекст с увеличенным таймаутом для надежности получения баланса
	balanceCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Получаем актуальный баланс токенов с максимальным уровнем подтверждения
	tokenBalance, err := d.GetTokenBalance(balanceCtx, rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("failed to get token balance: %w", err)
	}

	// Проверяем, что у пользователя есть токены для продажи
	if tokenBalance == 0 {
		return fmt.Errorf("no tokens to sell")
	}

	// TODO: здесь опять происходит расчет токенов
	// Рассчитываем количество токенов для продажи на основе процента
	tokensToSell := uint64(float64(tokenBalance) * (percentToSell / 100.0))

	// Логируем информацию о продаже
	d.logger.Info("Selling tokens",
		zap.String("token_mint", d.config.Mint.String()),
		zap.Uint64("total_balance", tokenBalance),
		zap.Float64("percent", percentToSell),
		zap.Uint64("tokens_to_sell", tokensToSell))

	// Выполняем продажу рассчитанного количества токенов
	return d.ExecuteSell(ctx, tokensToSell, slippagePercent, priorityFeeSol, computeUnits)
}
