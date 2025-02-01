// internal/dex/pumpfun/pumpfun.go
package pumpfun

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// DEX реализует интерфейс DEX для Pump.fun.
type DEX struct {
	client  *solbc.Client // Solana клиент для отправки транзакций.
	logger  *zap.Logger
	config  *PumpfunConfig       // Конфигурация Pump.fun.
	monitor *BondingCurveMonitor // Модуль мониторинга bonding curve.
	events  *PumpfunMonitor      // Монитор событий Pump.fun.
}

// NewDEX создает новый экземпляр Pump.fun DEX.
func NewDEX(client *solbc.Client, logger *zap.Logger, config *PumpfunConfig, monitorIntervalDuration string) (*DEX, error) {
	interval, err := parseDuration(monitorIntervalDuration)
	if err != nil {
		return nil, err
	}
	return &DEX{
		client:  client,
		logger:  logger.Named("pumpfun"),
		config:  config,
		monitor: NewBondingCurveMonitor(client, logger, interval),
		events:  NewPumpfunMonitor(logger, interval),
	}, nil
}

// parseDuration преобразует строковое значение в time.Duration.
func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// ExecuteSnipe выполняет быстрый снайпинг токена через Pump.fun.
func (d *DEX) ExecuteSnipe(ctx context.Context, tokenContract solana.PublicKey) error {
	d.logger.Info("Executing Pump.fun snipe", zap.String("contract", tokenContract.String()))

	// Формируем инструкцию создания токена через Pump.fun.
	createIx, err := BuildCreateTokenInstruction(d.config.ContractAddress, "PumpToken", "PUMP", "https://ipfs.io/ipfs/QmExample")
	if err != nil {
		return fmt.Errorf("failed to build create token instruction: %w", err)
	}

	// Отправляем транзакцию.
	txSig, err := d.client.CreateAndSendTransaction(ctx, []solana.Instruction{createIx})
	if err != nil {
		return fmt.Errorf("failed to send pumpfun snipe transaction: %w", err)
	}
	d.logger.Info("Pumpfun snipe transaction sent", zap.String("tx", txSig.String()))

	// Запускаем мониторинг bonding curve после снайпа.
	go d.monitor.Start(ctx)
	go d.events.Start(ctx)

	return nil
}

// ExecuteSell выполняет продажу токена через Pump.fun до достижения 100% bonding curve.
func (d *DEX) ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error {
	if !d.config.AllowSellBeforeFull {
		return fmt.Errorf("продажа не разрешена до достижения 100%% bonding curve")
	}
	d.logger.Info("Executing Pump.fun sell", zap.Uint64("amount", amount))

	// Получаем кошелек пользователя (предполагается, что клиент предоставляет этот метод).
	userWallet := d.client.GetWallet()

	// Собираем аккаунты для инструкции продажи.
	sellAccounts := SellInstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
	}

	sellIx, err := BuildSellTokenInstruction(sellAccounts, userWallet, amount, minSolOutput)
	if err != nil {
		return fmt.Errorf("не удалось сформировать инструкцию продажи токена: %w", err)
	}

	txSig, err := d.client.CreateAndSendTransaction(ctx, []solana.Instruction{sellIx})
	if err != nil {
		return fmt.Errorf("не удалось отправить транзакцию продажи: %w", err)
	}
	d.logger.Info("Pumpfun sell transaction sent", zap.String("tx", txSig.String()))
	return nil
}

// CheckForGraduation проверяет, достигла ли bonding curve порога graduation.
func (d *DEX) CheckForGraduation(ctx context.Context) (bool, error) {
	state, err := d.monitor.GetCurrentState()
	if err != nil {
		return false, err
	}
	d.logger.Debug("Bonding curve progress", zap.Float64("progress", state.Progress))
	return state.Progress >= d.config.GraduationThreshold, nil
}
