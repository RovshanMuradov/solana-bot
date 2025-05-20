// =============================
// File: internal/dex/pumpfun/pumpfun.go
// =============================
package pumpfun

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DEX представляет собой имплементацию интерфейса для взаимодействия с Pump.fun.
type DEX struct {
	client *blockchain.Client
	wallet *task.Wallet
	logger *zap.Logger
	config *Config

	// ---------- bonding‑curve cache ----------
	bcOnce                 sync.Once
	bondingCurve           solana.PublicKey
	associatedBondingCurve solana.PublicKey

	bcCache struct {
		mu        sync.RWMutex
		data      *BondingCurve
		fetchedAt time.Time
	}
}

// NewDEX создает новый экземпляр DEX для работы с Pump.fun.
func NewDEX(client *blockchain.Client, w *task.Wallet, logger *zap.Logger, config *Config, _ string) (*DEX, error) {
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
		client: client,
		wallet: w,
		logger: logger.Named("pumpfun"),
		config: config,
	}

	// Создаем контекст с таймаутом для получения данных о глобальном аккаунте
	fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Получаем информацию о глобальном аккаунте для определения получателя комиссий
	globalAccount, err := FetchGlobalAccount(fetchCtx, client, config.Global, logger)
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

// IsBondingCurveComplete проверяет, завершена ли bonding curve для токена.
// Возвращает true, если bonding curve завершена, иначе false.
func (d *DEX) IsBondingCurveComplete(ctx context.Context) (bool, error) {
	// Получаем данные bonding curve
	bc, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get bonding curve data: %w", err)
	}

	// Проверяем поле Complete
	return bc.Complete, nil
}
