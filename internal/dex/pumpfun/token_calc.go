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
func (d *DEX) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// Шаг 1: Вычисление адреса ассоциированного токен-аккаунта (ATA)
	mint := solana.MustPublicKeyFromBase58(tokenMint)
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, mint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Шаг 2: Запрос баланса токена с Processed commitment
	commitmentLevel := rpc.CommitmentProcessed
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)

	// Шаг 3: При неудаче с Processed, пробуем Confirmed
	if err != nil {
		d.logger.Debug("Failed to get balance with Processed commitment, trying Confirmed",
			zap.String("token_mint", tokenMint),
			zap.String("user_ata", userATA.String()),
			zap.Error(err))

		result, err = d.client.GetTokenAccountBalance(ctx, userATA, rpc.CommitmentConfirmed)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to get token account balance: %w", err)
	}

	// Шаг 4: Проверка результата на пустоту
	if result == nil || result.Value.Amount == "" {
		return 0, fmt.Errorf("no token balance found")
	}

	// Шаг 5: Преобразование строкового представления баланса в uint64
	balance, err := strconv.ParseUint(result.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token balance: %w", err)
	}

	d.logger.Debug("Got token balance",
		zap.Uint64("balance", balance),
		zap.String("token_mint", tokenMint),
		zap.String("user_ata", userATA.String()),
		zap.String("commitment", string(commitmentLevel)))

	return balance, nil
}

// calculateEstimate возвращает прогнозный выход SOL за tokenAmount,
// с учётом только протокольной комиссии.
func (d *DEX) calculateEstimate(
	ctx context.Context,
	tokenAmount float64,
	reserves interface{},
) (float64, error) {
	// Приводим reserves к типу BondingCurve
	bondingCurveData, ok := reserves.(*BondingCurve)
	if !ok {
		return 0, fmt.Errorf("invalid reserves type for pumpfun: %T", reserves)
	}

	// Проверка на нулевые резервы
	if bondingCurveData.VirtualTokenReserves == 0 || bondingCurveData.VirtualSolReserves == 0 {
		d.logger.Warn("Invalid reserve state with zero reserves",
			zap.Uint64("VirtualTokenReserves", bondingCurveData.VirtualTokenReserves),
			zap.Uint64("VirtualSolReserves", bondingCurveData.VirtualSolReserves))
		return 0, nil
	}

	// 1. Переводим tokenAmount в raw
	tokenAmountRaw := uint64(tokenAmount * math.Pow10(tokenDecimals))

	// 2. Считаем lamports через формулу AMM
	solAmountLamports := float64(tokenAmountRaw) * float64(bondingCurveData.VirtualSolReserves) /
		float64(bondingCurveData.VirtualTokenReserves+tokenAmountRaw)

	// 3. Учитываем комиссию протокола (1%)
	solAfterFees := solAmountLamports * (1.0 - (protocolFeePercent / 100.0))

	// 4. Конвертируем из lamports в SOL
	solOutput := solAfterFees / math.Pow10(solDecimals)

	d.logger.Debug("Calculated expected SOL output",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("token_amount_raw", float64(tokenAmountRaw)),
		zap.Float64("sol_amount_lamports_before_fee", solAmountLamports),
		zap.Float64("fee_percentage", protocolFeePercent),
		zap.Float64("sol_after_fees_lamports", solAfterFees),
		zap.Float64("sol_output", solOutput))

	return solOutput, nil
}

// calculateMinSolOutput вычисляет минимальный ожидаемый выход SOL при продаже токенов
// с учетом заданного допустимого проскальзывания.
// Сохранено для совместимости с trade.go.
func (d *DEX) calculateMinSolOutput(tokenAmount uint64, bondingCurveData *BondingCurve, slippagePercent float64) uint64 {
	// Формула из Python SDK: (tokens * virtual_sol_reserves) / (virtual_token_reserves + tokens)
	solAmount := (tokenAmount * bondingCurveData.VirtualSolReserves) /
		(bondingCurveData.VirtualTokenReserves + tokenAmount)

	// Применяем фиксированную комиссию протокола
	expectedSolValueLamports := uint64(float64(solAmount) * (1.0 - (protocolFeePercent / 100.0)))

	// Применяем допустимое проскальзывание
	slippageFactor := 1.0 - (slippagePercent / 100.0)
	return uint64(float64(expectedSolValueLamports) * slippageFactor)
}

// CalculatePnL вычисляет прибыль/убыток (PnL) для указанного количества токенов и начальной инвестиции.
// Расчет учитывает только комиссию протокола. Slippage не учитывается.
func (d *DEX) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	// 1. Учитываем buy-fee при вычислении costBasis
	buyFee := initialInvestment * (protocolFeePercent / 100)
	costBasis := initialInvestment - buyFee

	// 2. Получаем данные bonding curve
	bondingCurveData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		d.logger.Warn("Failed to fetch bonding curve data, assuming zero reserves", zap.Error(err))
		bondingCurveData = &BondingCurve{}
	}

	// 3. Рассчитываем ожидаемую выручку от продажи с учетом комиссии протокола
	sellEstimate, err := d.calculateEstimate(ctx, tokenAmount, bondingCurveData)
	if err != nil {
		d.logger.Warn("Error calculating sell estimate", zap.Error(err))
		sellEstimate = 0
	}

	// 4. Рассчитываем чистый PnL
	netPnL := sellEstimate - costBasis

	// 5. Рассчитываем процентный PnL
	pnlPercentage := 0.0
	if costBasis > 0 {
		pnlPercentage = (netPnL / costBasis) * 100
	} else if netPnL > 0 {
		// Если начальная инвестиция 0, а PnL положительный, это бесконечный процент
		pnlPercentage = math.Inf(1)
	}

	d.logger.Debug("PnL calculation completed",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("initial_investment", initialInvestment),
		zap.Float64("buy_fee", buyFee),
		zap.Float64("cost_basis", costBasis),
		zap.Float64("sell_estimate", sellEstimate),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	return &model.PnLResult{
		SellEstimate:      sellEstimate,
		InitialInvestment: costBasis,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}, nil
}

// SellPercentTokens продает указанный процент от доступного баланса токенов.
func (d *DEX) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	// Проверяем, что процент находится в допустимом диапазоне
	if percentToSell <= 0 || percentToSell > 100 {
		return fmt.Errorf("percent to sell must be between 0 and 100")
	}

	// Создаем контекст с увеличенным таймаутом для надежности получения баланса
	balanceCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Получаем актуальный баланс токенов
	tokenBalance, err := d.GetTokenBalance(balanceCtx, tokenMint)
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
		zap.String("token_mint", tokenMint),
		zap.Uint64("total_balance", tokenBalance),
		zap.Float64("percent", percentToSell),
		zap.Uint64("tokens_to_sell", tokensToSell))

	// Выполняем продажу рассчитанного количества токенов
	return d.ExecuteSell(ctx, tokensToSell, slippagePercent, priorityFeeSol, computeUnits)
}
