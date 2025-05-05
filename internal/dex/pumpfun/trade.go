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
func (d *DEX) prepareBuyTransaction(ctx context.Context, solAmountLamports uint64, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, error) {
	// Шаг 1: Подготавливаем базовые инструкции (установка приоритета и создание ATA)
	baseInstructions, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// Шаг 2: Получаем адреса аккаунтов bonding curve для токена
	bondingCurve, associatedBondingCurve, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, err
	}

	// Шаг 3: Создаем инструкцию покупки с точным количеством SOL
	buyIx := createBuyExactSolInstruction(
		d.config.Global,
		d.config.FeeRecipient,
		d.config.Mint,
		bondingCurve,
		associatedBondingCurve,
		userATA,
		d.wallet.PublicKey,
		d.config.EventAuthority,
		solAmountLamports,
	)

	// Шаг 4: Добавляем инструкцию покупки к базовым инструкциям
	baseInstructions = append(baseInstructions, buyIx)
	return baseInstructions, nil
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

	// базовые инструкции
	baseIx, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// адреса
	bondingCurve, associatedBC, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, err
	}

	// ----- берём данные Bonding‑Curve через кеш -----
	bcData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		d.logger.Warn("Failed to fetch bonding curve data", zap.Error(err))
	}

	// расчёт minSol
	minSolOutput := d.calculateMinSolOutput(tokenAmount, bcData, slippagePercent)

	// формируем sell‑инструкцию
	sellIx := createSellInstruction(
		d.config.ContractAddress,
		d.config.Global,
		d.config.FeeRecipient,
		d.config.Mint,
		bondingCurve,
		associatedBC,
		userATA,
		d.wallet.PublicKey,
		d.config.EventAuthority,
		tokenAmount,
		minSolOutput,
	)

	return append(baseIx, sellIx), nil
}

// calculateMinSolOutput вычисляет минимальный ожидаемый выход SOL при продаже токенов
// с учетом заданного допустимого проскальзывания.
func (d *DEX) calculateMinSolOutput(tokenAmount uint64, bondingCurveData *BondingCurve, slippagePercent float64) uint64 {
	// Вычисляем ожидаемый выход SOL на основе пропорции резервов в bonding curve
	// Формула: (токены * виртуальные резервы SOL) / виртуальные резервы токенов
	expectedSolValueLamports := (tokenAmount * bondingCurveData.VirtualSolReserves) / bondingCurveData.VirtualTokenReserves // TODO:  work with virtual balance

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
