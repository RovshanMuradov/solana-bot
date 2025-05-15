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
// Упрощено в соответствии с Python SDK.
func (d *DEX) prepareBuyTransaction(
	ctx context.Context,
	solAmountLamports uint64,
	priorityFeeSol string,
	computeUnits uint32,
) ([]solana.Instruction, error) {
	// 1) Базовые инструкции для приоритета и compute_unit_price
	baseInstructions, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// 2) Получаем все необходимые PDA и данные одновременно
	bcData, bcAddr, associatedBC, err := d.fetchBondingCurveAndDerivePDAs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare bonding curve data: %w", err)
	}

	// 3) Проверяем, нужно ли добавить extend_account
	info, err := d.client.GetAccountInfo(ctx, bcAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve info: %w", err)
	}

	if len(info.Value.Data.GetBinary()) < 150 {
		d.logger.Info("Adding extend_account instruction to transaction")
		extIx := createExtendAccountInstruction(
			bcAddr,
			d.wallet.PublicKey,
			d.config.EventAuthority,
			d.config.ContractAddress,
		)
		baseInstructions = append(baseInstructions, extIx)
	}

	// 4) Получаем creator vault, который зависит от Creator в bonding curve
	creatorVault, _, err := DeriveCreatorVaultPDA(d.config.ContractAddress, bcData.Creator)
	if err != nil {
		return nil, fmt.Errorf("failed to derive creator vault: %w", err)
	}
	d.logger.Info("Using creator vault", zap.String("vault", creatorVault.String()),
		zap.String("creator", bcData.Creator.String()))

	// 5) Формируем основную инструкцию buy
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

	// 6) Собираем и возвращаем все инструкции
	txIxs := append(baseInstructions, buyIx)
	return txIxs, nil
}

// prepareSellTransaction подготавливает транзакцию для продажи токенов на Pump.fun.
// Упрощено в соответствии с Python SDK.
func (d *DEX) prepareSellTransaction(
	ctx context.Context,
	tokenAmount uint64,
	slippagePercent float64,
	priorityFeeSol string,
	computeUnits uint32,
) ([]solana.Instruction, error) {
	// 1) Базовые инструкции для приоритета и compute_unit_price
	baseIxs, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// 2) Получаем все необходимые PDA и данные одновременно
	bcData, bondingCurve, associatedBC, err := d.fetchBondingCurveAndDerivePDAs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare bonding curve data: %w", err)
	}

	// 3) Проверяем, нужно ли добавить extend_account
	info, err := d.client.GetAccountInfo(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve info: %w", err)
	}

	if len(info.Value.Data.GetBinary()) < 150 {
		d.logger.Info("Adding extend_account instruction to sell transaction")
		extIx := createExtendAccountInstruction(
			bondingCurve,
			d.wallet.PublicKey,
			d.config.EventAuthority,
			d.config.ContractAddress,
		)
		baseIxs = append(baseIxs, extIx)
	}

	// 4) Получаем creator vault, который зависит от Creator в bonding curve
	creatorVault, _, err := DeriveCreatorVaultPDA(d.config.ContractAddress, bcData.Creator)
	if err != nil {
		return nil, fmt.Errorf("failed to derive creator vault: %w", err)
	}
	d.logger.Info("Using creator vault for sell", zap.String("vault", creatorVault.String()),
		zap.String("creator", bcData.Creator.String()))

	// 5) Рассчитываем минимальный выход SOL с учётом слиппэджа
	minSolOutput := d.calculateMinSolOutput(tokenAmount, bcData, slippagePercent)

	// 6) Формируем sell-инструкцию
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

	// 7) Собираем и возвращаем все инструкции
	return append(baseIxs, sellIx), nil
}

// fetchBondingCurveAndDerivePDAs получает Bonding Curve данные и все необходимые PDA за один вызов.
// Также проверяет необходимость extend_account.
func (d *DEX) fetchBondingCurveAndDerivePDAs(ctx context.Context) (*BondingCurve, solana.PublicKey, solana.PublicKey, error) {
	// 1) Деривируем PDA bonding curve и associated token account
	bcAddr, associatedBC, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, solana.PublicKey{}, solana.PublicKey{}, err
	}

	// 2) Проверяем размер данных, при необходимости extend_account
	info, err := d.client.GetAccountInfo(ctx, bcAddr)
	if err != nil {
		return nil, solana.PublicKey{}, solana.PublicKey{}, err
	}

	// Проверяем размер - если данные слишком маленькие (старая версия аккаунта)
	// Необходимо выполнить extend_account
	if len(info.Value.Data.GetBinary()) < 150 {
		d.logger.Warn("Bonding curve account needs extension",
			zap.String("bonding_curve", bcAddr.String()),
			zap.Int("current_size", len(info.Value.Data.GetBinary())),
			zap.Int("required_size", 150))

		// Возвращаем информацию, чтобы вызывающий код мог добавить инструкцию
		// extend_account в транзакцию buy/sell вместо отдельной транзакции
	}

	// 3) Получаем данные bonding curve
	bcData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		return nil, solana.PublicKey{}, solana.PublicKey{}, err
	}

	return bcData, bcAddr, associatedBC, nil
}

// calculateMinSolOutput вычисляет минимальный ожидаемый выход SOL при продаже токенов
// с учетом заданного допустимого проскальзывания. Упрощено в соответствии с Python SDK.
func (d *DEX) calculateMinSolOutput(tokenAmount uint64, bondingCurveData *BondingCurve, slippagePercent float64) uint64 {
	// Проверка на нулевые резервы, как в Python-коде
	if bondingCurveData.VirtualTokenReserves == 0 || bondingCurveData.VirtualSolReserves == 0 {
		d.logger.Warn("Invalid reserve state with zero reserves",
			zap.Uint64("VirtualTokenReserves", bondingCurveData.VirtualTokenReserves),
			zap.Uint64("VirtualSolReserves", bondingCurveData.VirtualSolReserves))
		return 0
	}

	// Формула из Python SDK: (tokens * virtual_sol_reserves) / (virtual_token_reserves + tokens)
	// Это прямое соответствие формуле из Python кода
	solAmount := (tokenAmount * bondingCurveData.VirtualSolReserves) /
		(bondingCurveData.VirtualTokenReserves + tokenAmount)

	// Применяем фиксированные комиссии - упрощенный подход
	feePercentage := 1.0 // Базовая комиссия протокола 1%
	if bondingCurveData.Creator != (solana.PublicKey{}) {
		feePercentage += 0.0 // В текущий момент creator_fee = 0, но можно добавить в будущем
	}

	// Учитываем комиссию
	expectedSolValueLamports := uint64(float64(solAmount) * (1.0 - (feePercentage / 100.0)))

	// Логируем расчет в упрощенном виде
	d.logger.Debug("Calculated min SOL output (simplified)",
		zap.Uint64("token_amount", tokenAmount),
		zap.Uint64("sol_amount_before_fee", solAmount),
		zap.Float64("fee_percentage", feePercentage),
		zap.Uint64("expected_sol_after_fee", expectedSolValueLamports),
		zap.Float64("slippage_percent", slippagePercent))

	// Применяем допустимое проскальзывание, как в Python-коде
	slippageFactor := 1.0 - (slippagePercent / 100.0)
	return uint64(float64(expectedSolValueLamports) * slippageFactor)
}

// handleSellError обрабатывает ошибки, возникающие при продаже токенов.
// Упрощено в соответствии с подходом Python SDK.
func (d *DEX) handleSellError(err error) error {
	// Базовое сообщение об ошибке
	errMsg := fmt.Sprintf("Ошибка при продаже токена %s: %s",
		d.config.Mint.String(), err.Error())

	// Логируем ошибку
	d.logger.Error("Sell transaction failed",
		zap.String("token_mint", d.config.Mint.String()),
		zap.Error(err))

	// Проверяем наличие специфических сообщений об ошибках
	if strings.Contains(err.Error(), "BondingCurveComplete") ||
		strings.Contains(err.Error(), "0x1775") ||
		strings.Contains(err.Error(), "6005") {
		return fmt.Errorf("%s. Токен перенесен на Raydium", errMsg)
	}

	return fmt.Errorf("%s. Попробуйте изменить параметры транзакции", errMsg)
}
