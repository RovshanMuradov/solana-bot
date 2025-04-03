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
//
// Метод формирует полный набор инструкций для транзакции покупки токенов за точное количество SOL.
// Процесс включает подготовку приоритетных инструкций, создание и проверку ассоциированных токен-аккаунтов,
// и генерацию инструкции покупки на основе адресов bonding curve.
//
// Параметры:
//   - ctx: контекст выполнения с возможностью отмены или таймаута
//   - solAmountLamports: точное количество SOL в ламппортах для покупки (1 SOL = 10^9 ламппортов)
//   - priorityFeeSol: приоритетная комиссия в SOL (в строковом формате) для ускорения транзакции
//   - computeUnits: количество вычислительных единиц для транзакции
//
// Возвращает:
//   - []solana.Instruction: массив инструкций для включения в транзакцию
//   - error: ошибку, если не удалось подготовить инструкции
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
		d.config.Global,         // Глобальный аккаунт протокола
		d.config.FeeRecipient,   // Аккаунт для получения комиссий
		d.config.Mint,           // Адрес минта токена
		bondingCurve,            // Аккаунт bonding curve
		associatedBondingCurve,  // Ассоциированный токен-аккаунт для bonding curve
		userATA,                 // Ассоциированный токен-аккаунт пользователя
		d.wallet.PublicKey,      // Публичный ключ кошелька пользователя
		d.config.EventAuthority, // Аккаунт для событий протокола
		solAmountLamports,       // Точное количество SOL в ламппортах
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
//
// Параметры:
//   - ctx: контекст выполнения с возможностью отмены или таймаута
//   - tokenAmount: количество токенов для продажи (в минимальных единицах токена)
//   - slippagePercent: максимально допустимое проскальзывание в процентах (0-100)
//   - priorityFeeSol: приоритетная комиссия в SOL для ускорения транзакции
//   - computeUnits: количество вычислительных единиц для транзакции
//
// Возвращает:
//   - []solana.Instruction: массив инструкций для включения в транзакцию
//   - error: ошибку, если не удалось подготовить инструкции, например, если
//     bonding curve завершена или перемещена на другой DEX
func (d *DEX) prepareSellTransaction(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, error) {
	// Шаг 1: Подготавливаем базовые инструкции (установка приоритета и проверка ATA)
	baseInstructions, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// Шаг 2: Получаем адреса аккаунтов bonding curve для токена
	bondingCurve, associatedBondingCurve, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, err
	}

	// Шаг 3: Получаем данные аккаунта bonding curve для расчета цены
	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bonding curve data: %w", err)
	}

	// Шаг 4: Проверяем, не завершена ли bonding curve (перемещена на другой DEX)
	// Если резервы SOL слишком малы, возможно, токен переехал на Raydium
	if bondingCurveData.VirtualSolReserves < 1000 {
		return nil, fmt.Errorf("bonding curve has insufficient SOL reserves, possibly complete")
	}

	// Шаг 5: Рассчитываем минимальный ожидаемый выход SOL с учетом проскальзывания
	minSolOutput := d.calculateMinSolOutput(tokenAmount, bondingCurveData, slippagePercent)

	// Шаг 6: Логируем параметры продажи для отладки
	d.logger.Info("Calculated sell parameters",
		zap.Uint64("token_amount", tokenAmount),
		zap.Uint64("virtual_token_reserves", bondingCurveData.VirtualTokenReserves),
		zap.Uint64("virtual_sol_reserves", bondingCurveData.VirtualSolReserves),
		zap.Uint64("min_sol_output_lamports", minSolOutput))

	// Шаг 7: Создаем инструкцию продажи с расчетными параметрами
	sellIx := createSellInstruction(
		d.config.ContractAddress, // ID программы Pump.fun
		d.config.Global,          // Глобальный аккаунт протокола
		d.config.FeeRecipient,    // Аккаунт для получения комиссий
		d.config.Mint,            // Адрес минта токена
		bondingCurve,             // Аккаунт bonding curve
		associatedBondingCurve,   // Ассоциированный токен-аккаунт для bonding curve
		userATA,                  // Ассоциированный токен-аккаунт пользователя
		d.wallet.PublicKey,       // Публичный ключ кошелька пользователя
		d.config.EventAuthority,  // Аккаунт для событий протокола
		tokenAmount,              // Количество токенов для продажи
		minSolOutput,             // Минимальный выход SOL с учетом проскальзывания
	)

	// Шаг 8: Добавляем инструкцию продажи к базовым инструкциям
	baseInstructions = append(baseInstructions, sellIx)
	return baseInstructions, nil
}

// calculateMinSolOutput вычисляет минимальный ожидаемый выход SOL при продаже токенов
// с учетом заданного допустимого проскальзывания.
//
// Метод использует формулу bonding curve для расчета ожидаемого количества SOL
// и применяет проскальзывание для защиты пользователя от неблагоприятного изменения цены.
//
// Параметры:
//   - tokenAmount: количество токенов для продажи (в минимальных единицах)
//   - bondingCurveData: данные аккаунта bonding curve, содержащие информацию о резервах
//   - slippagePercent: максимально допустимое проскальзывание в процентах (0-100)
//
// Возвращает:
//   - uint64: минимальное количество SOL в ламппортах, которое пользователь готов принять
func (d *DEX) calculateMinSolOutput(tokenAmount uint64, bondingCurveData *BondingCurve, slippagePercent float64) uint64 {
	// Вычисляем ожидаемый выход SOL на основе пропорции резервов в bonding curve
	// Формула: (токены * виртуальные резервы SOL) / виртуальные резервы токенов
	expectedSolValueLamports := (tokenAmount * bondingCurveData.VirtualSolReserves) / bondingCurveData.VirtualTokenReserves

	// Применяем допустимое проскальзывание к ожидаемому выходу
	// Например, при проскальзывании 1% получим 99% от ожидаемого значения
	return uint64(float64(expectedSolValueLamports) * (1.0 - slippagePercent/100.0))
}

// handleSellError обрабатывает специальные ошибки, возникающие при продаже токенов.
//
// Метод распознает и преобразует специфические ошибки смарт-контракта в более
// понятные сообщения. В частности, он определяет ситуацию, когда токен переехал
// с Pump.fun на другой DEX (обычно Raydium).
//
// Параметры:
//   - err: исходная ошибка, полученная при выполнении транзакции
//
// Возвращает:
//   - error: обработанную ошибку с более информативным сообщением
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
