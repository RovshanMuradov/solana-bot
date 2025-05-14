// =============================
// File: internal/dex/pumpfun/trade.go
// =============================
package pumpfun

import (
	"context"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// prepareBuyTransaction подготавливает транзакцию для покупки токенов на Pump.fun.
func (d *DEX) prepareBuyTransaction(
	ctx context.Context,
	solAmountLamports uint64,
	priorityFeeSol string,
	computeUnits uint32,
) ([]solana.Instruction, error) {
	// 1) Базовые инструкции: fee, compute budget, создание ATA
	baseInstructions, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// 2) Деривируем PDA bondingCurve
	bcAddr, associatedBC, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, err
	}

	// 3) Проверяем размер данных bondingCurve и, при необходимости, extend_account
	info, err := d.client.GetAccountInfo(ctx, bcAddr)
	if err != nil {
		return nil, err
	}
	if len(info.Value.Data.GetBinary()) < 150 {
		extIx := createExtendAccountInstruction(
			bcAddr,
			d.wallet.PublicKey,
			d.config.EventAuthority,
			d.config.ContractAddress,
		)
		baseInstructions = append(baseInstructions, extIx)
	}

	// 4) Получаем полную структуру bondingCurve, чтобы вытащить Creator
	bcData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		return nil, err
	}

	// 5) Деривим PDA creator_vault по seeds ["creator-vault", creator]
	creatorVault, _, err := DeriveCreatorVaultPDA(d.config.ContractAddress, bcData.Creator)
	if err != nil {
		return nil, fmt.Errorf("failed to derive creator vault PDA for buy: %w", err)
	}

	// 6) Формируем основную инструкцию buy, передавая creatorVault
	buyIx := createBuyExactSolInstruction(
		d.config.Global,
		d.config.FeeRecipient,
		d.config.Mint,
		bcAddr,
		associatedBC,
		userATA,
		d.wallet.PublicKey,
		creatorVault,
		d.config.EventAuthority,
		d.config.ContractAddress,
		solAmountLamports,
	)

	// 7) Собираем и возвращаем все инструкции
	txIxs := append(baseInstructions, buyIx)
	return txIxs, nil
}

// prepareSellTransaction подготавливает транзакцию для продажи токенов на Pump.fun.
//
// Метод формирует полный набор инструкций для транзакции продажи токенов.
// Он выполняет проверку состояния bonding curve, рассчитывает минимальный ожидаемый выход SOL
// с учетом проскальзывания, и создает инструкцию продажи.
func (d *DEX) prepareSellTransaction(
	ctx context.Context,
	tokenAmount uint64,
	slippagePercent float64,
	priorityFeeSol string,
	computeUnits uint32,
) ([]solana.Instruction, error) {
	// 1) Базовые инструкции: fee, compute budget, создание ATA
	baseIxs, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// 2) Деривация PDA bondingCurve и associated token account
	bondingCurve, associatedBC, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, err
	}

	// 3) Проверяем размер данных bondingCurve и, при необходимости, extend_account
	info, err := d.client.GetAccountInfo(ctx, bondingCurve)
	if err != nil {
		return nil, err
	}
	if len(info.Value.Data.GetBinary()) < 150 {
		extIx := createExtendAccountInstruction(
			bondingCurve,
			d.wallet.PublicKey,
			d.config.EventAuthority,
			d.config.ContractAddress,
		)
		baseIxs = append(baseIxs, extIx)
	}

	// 4) Получаем полную структуру bondingCurve, чтобы вытащить Creator
	bcData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		return nil, err
	}

	// 5) Деривим PDA creator_vault по seeds ["creator-vault", creator]
	creatorVault, _, err := DeriveCreatorVaultPDA(d.config.ContractAddress, bcData.Creator)
	if err != nil {
		return nil, fmt.Errorf("failed to derive creator vault PDA for sell: %w", err)
	}

	// 6) Рассчитываем минимальный выход SOL с учётом слиппэджа
	minSolOutput := d.calculateMinSolOutput(tokenAmount, bcData, slippagePercent)

	// 7) Формируем sell-инструкцию с новым creator_vault
	sellIx := createSellInstruction(
		d.config.ContractAddress,
		d.config.Global,
		d.config.FeeRecipient,
		d.config.Mint,
		bondingCurve,
		associatedBC,
		userATA,
		d.wallet.PublicKey,
		creatorVault,
		d.config.EventAuthority,
		tokenAmount,
		minSolOutput,
	)

	// 8) Собираем и возвращаем все инструкции
	return append(baseIxs, sellIx), nil
}

// calculateMinSolOutput вычисляет минимальный ожидаемый выход SOL при продаже токенов
// с учетом заданного допустимого проскальзывания и комиссий (включая новую creator_fee).
func (d *DEX) calculateMinSolOutput(tokenAmount uint64, bondingCurveData *BondingCurve, slippagePercent float64) uint64 {
	// Получаем глобальные настройки для расчёта комиссий
	ctx := context.Background()
	globalAccount, err := FetchGlobalAccount(ctx, d.client, d.config.Global, d.logger)
	if err != nil {
		d.logger.Warn("Failed to fetch global account for fee calculation, using default fee values",
			zap.Error(err))
		// Создаём заглушку с базовыми значениями при ошибке
		globalAccount = &GlobalAccount{
			FeeBasisPoints:        100, // дефолтные 1%
			CreatorFeeBasisPoints: 0,   // дефолтные 0%
		}
	}

	// Вычисляем ожидаемый выход SOL на основе пропорции резервов в bonding curve
	// Формула: (токены * виртуальные резервы SOL) / (виртуальные резервы токенов + токены)
	solAmount := (tokenAmount * bondingCurveData.VirtualSolReserves) / (bondingCurveData.VirtualTokenReserves + tokenAmount)

	// Вычитаем комиссии (протокол + creator fee)
	totalFee := computeTotalFee(globalAccount, bondingCurveData, solAmount, false)
	expectedSolValueLamports := solAmount - totalFee

	// Применяем допустимое проскальзывание к ожидаемому выходу
	// Например, при проскальзывании 1% получим 99% от ожидаемого значения
	return uint64(float64(expectedSolValueLamports) * (1.0 - slippagePercent/100.0))
}

// TODO: рассмотреть удаление или изменение метода
// handleSellError обрабатывает специальные ошибки, возникающие при продаже токенов.
func (d *DEX) handleSellError(err error) error {
	// Проверяем специфические коды ошибок или строки, которые указывают на то,
	// что bonding curve завершена или токен переехал на другой DEX
	if strings.Contains(err.Error(), "BondingCurveComplete") ||
		strings.Contains(err.Error(), "0x1775") ||
		strings.Contains(err.Error(), "6005") {
		// Логируем детали ошибки
		d.logger.Error("Невозможно продать токен через Pump.fun",
			zap.String("token_mint", d.config.Mint.String()),
			zap.String("reason", "Токен перенесен на Raydium"))

		// Возвращаем понятное сообщение об ошибке с объяснением
		return fmt.Errorf("токен %s перенесен на Raydium и не может быть продан через Pump.fun",
			d.config.Mint.String())
	}

	// Если это не специфическая ошибка, добавляем общее сообщение
	return fmt.Errorf("failed to send transaction: %w", err)
}
