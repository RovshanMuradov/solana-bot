// internal/dex/pumpfun/token_calc.go
package pumpfun

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"math"
	"strconv"
	"time"

	"go.uber.org/zap"
)

const (
	// Стандартные десятичные знаки для SOL и токенов Pump.fun
	solDecimals   = 9
	tokenDecimals = 6
	// Минимальная цена для предотвращения деления на ноль или слишком малых значений
	minPriceThreshold = 1e-18 // Очень маленькое значение, близкое к нулю
	// Базовая комиссия протокола Pump.fun
	protocolFeePercent = 1.0 // 1% комиссия протокола
	// Стандартное проскальзывание для расчетов
	defaultSlippagePercent = 0.5 // 0.5% проскальзывание по умолчанию
)

// CalculateTokenPrice рассчитывает текущую спотовую цену токена на основе виртуальных резервов bonding curve.
// Формула: Price = (VirtualSolReserves / 10^solDecimals) / (VirtualTokenReserves / 10^tokenDecimals)
// Возвращает текущую цену токена в SOL и ошибку, если расчет не удался.
func (d *DEX) CalculateTokenPrice(ctx context.Context, bondingCurveData *BondingCurve) (float64, error) {
	if bondingCurveData == nil {
		var err error
		bondingCurveData, _, err = d.getBondingCurveData(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get bonding curve data: %w", err)
		}
	}

	// Проверка на нулевые резервы
	if bondingCurveData.VirtualTokenReserves == 0 || bondingCurveData.VirtualSolReserves == 0 {
		d.logger.Warn("Invalid reserve state with zero reserves",
			zap.Uint64("VirtualTokenReserves", bondingCurveData.VirtualTokenReserves),
			zap.Uint64("VirtualSolReserves", bondingCurveData.VirtualSolReserves))
		return minPriceThreshold, fmt.Errorf("bonding curve has zero reserves")
	}

	// Конвертация виртуальных резервов из lamports/минимальных единиц в полные единицы (SOL и токены)
	virtualSolFloat := float64(bondingCurveData.VirtualSolReserves) / math.Pow10(solDecimals)
	virtualTokenFloat := float64(bondingCurveData.VirtualTokenReserves) / math.Pow10(tokenDecimals)

	// Расчет цены
	price := virtualSolFloat / virtualTokenFloat

	// Применяем нижнюю границу цены для предотвращения слишком малых значений
	if price < minPriceThreshold {
		d.logger.Debug("Calculated price below minimum threshold, adjusting",
			zap.Float64("raw_price", price),
			zap.Float64("min_threshold", minPriceThreshold))
		price = minPriceThreshold
	}

	d.logger.Debug("Calculated token spot price using virtual reserves ratio",
		zap.Uint64("virtual_sol_lamports", bondingCurveData.VirtualSolReserves),
		zap.Uint64("virtual_token_units", bondingCurveData.VirtualTokenReserves),
		zap.Float64("calculated_price_sol_per_token", price))

	return price, nil
}

// GetTokenPrice возвращает текущую цену токена по его минту, используя данные bonding curve.
// Этот метод является публичным API для внешних вызовов из адаптеров и интерфейсов верхнего уровня.
// Метод проверяет валидность минта и возвращает текущую цену токена в SOL.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Проверяем, что запрашиваем именно тот минт, для которого инициализировали DEX
	if d.config.Mint.String() != tokenMint {
		return 0, fmt.Errorf("token %s not configured in this DEX instance", tokenMint)
	}

	// Берём данные кривой из внутреннего TTL‑кэша
	bcData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get bonding curve data: %w", err)
	}

	// Если кривая «пустая» — значит токен уже уехал на Raydium/PumpSwap
	if bcData.VirtualTokenReserves == 0 || bcData.VirtualSolReserves == 0 {
		d.logger.Warn("Bonding curve may be graduated or not available",
			zap.String("token_mint", tokenMint))
		return 0, fmt.Errorf("bonding curve for token %s is graduated or not available", tokenMint)
	}

	// Используем внутренний метод для расчета цены
	return d.CalculateTokenPrice(ctx, bcData)
}

// GetTokenBalance возвращает текущий баланс токена в кошельке пользователя.
// Метод определяет ассоциированный токен-аккаунт для кошелька и запрашивает его баланс.
// Сначала пытается получить баланс с использованием быстрого уровня подтверждения Processed,
// при неудаче переключается на более надежный уровень Confirmed.
func (d *DEX) GetTokenBalance(ctx context.Context, commitment ...rpc.CommitmentType) (uint64, error) {
	// Шаг 1: Вычисление адреса ассоциированного токен-аккаунта (ATA)
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Шаг 2: Определение уровня подтверждения (commitment level)
	commitmentLevel := rpc.CommitmentProcessed
	if len(commitment) > 0 {
		commitmentLevel = commitment[0]
	}

	// Шаг 3: Запрос баланса токена с выбранным уровнем подтверждения
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)

	// Шаг 4: При неудаче с Processed, пробуем Confirmed
	if err != nil && commitmentLevel == rpc.CommitmentProcessed {
		d.logger.Debug("Failed to get balance with Processed commitment, trying Confirmed",
			zap.String("token_mint", d.config.Mint.String()),
			zap.String("user_ata", userATA.String()),
			zap.Error(err))

		result, err = d.client.GetTokenAccountBalance(ctx, userATA, rpc.CommitmentConfirmed)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to get token account balance: %w", err)
	}

	// Шаг 5: Проверка результата на пустоту
	if result == nil || result.Value.Amount == "" {
		return 0, fmt.Errorf("no token balance found")
	}

	// Шаг 6: Преобразование строкового представления баланса в uint64
	balance, err := strconv.ParseUint(result.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token balance: %w", err)
	}

	d.logger.Debug("Got token balance",
		zap.Uint64("balance", balance),
		zap.String("token_mint", d.config.Mint.String()),
		zap.String("user_ata", userATA.String()),
		zap.String("commitment", string(commitmentLevel)))

	return balance, nil
}

// calculateExpectedSolOutput вычисляет ожидаемый выход SOL при продаже токенов
// с учетом формулы bonding curve, комиссий протокола и проскальзывания.
// Формула: (tokens * virtual_sol_reserves) / (virtual_token_reserves + tokens)
func (d *DEX) calculateExpectedSolOutput(tokenAmount float64, bondingCurveData *BondingCurve, slippagePercent float64) float64 {
	// Проверка на нулевые резервы
	if bondingCurveData.VirtualTokenReserves == 0 || bondingCurveData.VirtualSolReserves == 0 {
		d.logger.Warn("Invalid reserve state with zero reserves",
			zap.Uint64("VirtualTokenReserves", bondingCurveData.VirtualTokenReserves),
			zap.Uint64("VirtualSolReserves", bondingCurveData.VirtualSolReserves))
		return 0
	}

	// Преобразуем токены в минимальные единицы
	tokenAmountRaw := uint64(tokenAmount * math.Pow10(tokenDecimals))

	// Формула bonding curve для расчета выхода SOL в lamports
	solAmountLamports := float64(tokenAmountRaw) * float64(bondingCurveData.VirtualSolReserves) /
		float64(bondingCurveData.VirtualTokenReserves+tokenAmountRaw)

	// Применяем фиксированные комиссии
	feePercentage := protocolFeePercent // Базовая комиссия протокола
	// Проверка на наличие комиссии создателя
	// В текущий момент creator_fee = 0, но в будущем может потребоваться добавить:
	// if bondingCurveData.Creator != (solana.PublicKey{}) {
	//     feePercentage += creatorFeePercent
	// }

	// Учитываем комиссию
	solAfterFees := solAmountLamports * (1.0 - (feePercentage / 100.0))

	// Применяем допустимое проскальзывание
	slippageFactor := 1.0 - (slippagePercent / 100.0)
	finalSolAmount := solAfterFees * slippageFactor

	// Конвертируем из lamports в SOL
	solOutput := finalSolAmount / math.Pow10(solDecimals)

	d.logger.Debug("Calculated expected SOL output with slippage",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("token_amount_raw", float64(tokenAmountRaw)),
		zap.Float64("sol_amount_lamports_before_fee", solAmountLamports),
		zap.Float64("fee_percentage", feePercentage),
		zap.Float64("sol_after_fees_lamports", solAfterFees),
		zap.Float64("slippage_percent", slippagePercent),
		zap.Float64("final_sol_lamports", finalSolAmount),
		zap.Float64("sol_output", solOutput))

	return solOutput
}

// calculateMinSolOutput вычисляет минимальный ожидаемый выход SOL при продаже токенов
// с учетом заданного допустимого проскальзывания.
// Этот метод сохранен для обратной совместимости и используется в trade.go.
func (d *DEX) calculateMinSolOutput(tokenAmount uint64, bondingCurveData *BondingCurve, slippagePercent float64) uint64 {
	// Проверка на нулевые резервы
	if bondingCurveData.VirtualTokenReserves == 0 || bondingCurveData.VirtualSolReserves == 0 {
		d.logger.Warn("Invalid reserve state with zero reserves",
			zap.Uint64("VirtualTokenReserves", bondingCurveData.VirtualTokenReserves),
			zap.Uint64("VirtualSolReserves", bondingCurveData.VirtualSolReserves))
		return 0
	}

	// Формула из Python SDK: (tokens * virtual_sol_reserves) / (virtual_token_reserves + tokens)
	solAmount := (tokenAmount * bondingCurveData.VirtualSolReserves) /
		(bondingCurveData.VirtualTokenReserves + tokenAmount)

	// Применяем фиксированные комиссии
	feePercentage := protocolFeePercent // Базовая комиссия протокола 1%
	// Проверка на наличие комиссии создателя
	// В текущий момент creator_fee = 0, но в будущем может потребоваться добавить:
	// if bondingCurveData.Creator != (solana.PublicKey{}) {
	//     feePercentage += creatorFeePercent
	// }

	// Учитываем комиссию
	expectedSolValueLamports := uint64(float64(solAmount) * (1.0 - (feePercentage / 100.0)))

	// Логируем расчет
	d.logger.Debug("Calculated min SOL output",
		zap.Uint64("token_amount", tokenAmount),
		zap.Uint64("sol_amount_before_fee", solAmount),
		zap.Float64("fee_percentage", feePercentage),
		zap.Uint64("expected_sol_after_fee", expectedSolValueLamports),
		zap.Float64("slippage_percent", slippagePercent))

	// Применяем допустимое проскальзывание
	slippageFactor := 1.0 - (slippagePercent / 100.0)
	return uint64(float64(expectedSolValueLamports) * slippageFactor)
}

// CalculatePnL вычисляет PnL (прибыль/убыток) для указанного количества токенов и начальной инвестиции.
// Расчет учитывает:
// 1. Текущую спотовую цену токена на основе данных bonding curve
// 2. Теоретическую стоимость (количество токенов * текущая цена)
// 3. Ожидаемую выручку от продажи с учетом комиссий и проскальзывания
// 4. Чистый PnL как разницу между ожидаемой выручкой и начальной инвестицией
// 5. Процентный PnL относительно начальной инвестиции
func (d *DEX) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	// Получаем данные bonding curve
	bondingCurveData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		d.logger.Warn("Failed to fetch bonding curve data, assuming zero reserves", zap.Error(err))
		bondingCurveData = &BondingCurve{}
	}

	// 1. Рассчитываем текущую спотовую цену токена
	currentPrice, err := d.CalculateTokenPrice(ctx, bondingCurveData)
	if err != nil {
		d.logger.Warn("Error calculating token price, using minimum threshold", zap.Error(err))
		currentPrice = minPriceThreshold
	}

	// 2. Рассчитываем теоретическую стоимость (без учета комиссий и проскальзывания)
	theoreticalValue := tokenAmount * currentPrice

	// 3. Рассчитываем ожидаемую выручку от продажи с учетом комиссий и проскальзывания
	sellEstimate := d.calculateExpectedSolOutput(tokenAmount, bondingCurveData, defaultSlippagePercent)

	// 4. Рассчитываем чистый PnL
	netPnL := sellEstimate - initialInvestment

	// 5. Рассчитываем процентный PnL
	pnlPercentage := 0.0
	if initialInvestment > 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100
	} else if netPnL > 0 {
		// Если начальная инвестиция 0, а PnL положительный, это бесконечный процент
		pnlPercentage = math.Inf(1)
	}

	d.logger.Debug("PnL calculation completed",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("initial_investment", initialInvestment),
		zap.Float64("current_price", currentPrice),
		zap.Float64("theoretical_value", theoreticalValue),
		zap.Float64("sell_estimate_with_fees_and_slippage", sellEstimate),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	return &model.PnLResult{
		SellEstimate:      sellEstimate,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}, nil
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
