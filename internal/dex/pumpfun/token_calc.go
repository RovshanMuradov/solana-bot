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

	"go.uber.org/zap"
)

const (
	// Стандартные десятичные знаки для SOL и токенов Pump.fun
	solDecimals   = 9
	tokenDecimals = 6
	// Минимальная цена для предотвращения деления на ноль или слишком малых значений
	minPriceThreshold = 1e-18 // Очень маленькое значение, близкое к нулю
)

// CalculateTokenPrice рассчитывает текущую спотовую цену токена на основе виртуальных резервов bonding curve.
// Формула: Price = (VirtualSolReserves / 10^solDecimals) / (VirtualTokenReserves / 10^tokenDecimals)
// Эта формула является аппроксимацией и может отличаться от точной математической модели кривой Pump.fun.
func (d *DEX) CalculateTokenPrice(ctx context.Context, bondingCurveData *BondingCurve) (float64, error) {
	if bondingCurveData == nil {
		var err error
		bondingCurveData, _, err = d.getBondingCurveData(ctx)
		if err != nil {
			return 0, err
		}
	}

	// Конвертация виртуальных резервов из lamports/минимальных единиц в полные единицы (SOL и токены)
	virtualSolFloat := float64(bondingCurveData.VirtualSolReserves) / math.Pow10(solDecimals)
	virtualTokenFloat := float64(bondingCurveData.VirtualTokenReserves) / math.Pow10(tokenDecimals)

	// Расчет цены
	price := virtualSolFloat / virtualTokenFloat

	// Применяем нижнюю границу цены
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

// CalculateSellValue вычисляет оценку SOL (выручку) от продажи заданного количества токенов,
// Она просто умножает количество токенов на текущую цену.
func (d *DEX) CalculateSellValue(ctx context.Context, tokenAmount float64, bondingCurveData *BondingCurve) (float64, error) {
	if bondingCurveData == nil {
		return 0, fmt.Errorf("bonding curve data is nil")
	}

	// Получаем текущую спотовую цену по модели bonding curve
	currentPrice, err := d.CalculateTokenPrice(ctx, bondingCurveData)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate current price: %w", err)
	}

	// Базовая теоретическая стоимость продажи (без учета комиссии и slippage)
	baseValue := tokenAmount * currentPrice

	d.logger.Debug("Sell estimate calculation (slippage NOT included)",
		zap.Float64("tokenAmount", tokenAmount),
		zap.Float64("currentPrice", currentPrice),
		zap.Float64("baseValue", baseValue))

	// Дополнительное логирование, если оценка продажи равна нулю
	if baseValue <= 0 {
		d.logger.Warn("Sell estimate is zero or negative",
			zap.Float64("tokenAmount", tokenAmount),
			zap.Float64("currentPrice", currentPrice))
	}

	return baseValue, nil
}

// CalculatePnL вычисляет PnL (прибыль/убыток) на основе модели bonding curve.
// Расчет учитывает виртуальные резервы токена и SOL, применяет комиссию протокола,
// но НЕ учитывает проскальзывание (slippage) при больших объемах продажи.
func (d *DEX) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	bondingCurveData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		d.logger.Warn("Failed to fetch bonding curve data, assuming zero reserves", zap.Error(err))
		bondingCurveData = &BondingCurve{}
	}

	currentPrice, err := d.CalculateTokenPrice(ctx, bondingCurveData)
	// Не возвращаем ошибку здесь, так как CalculateTokenPrice может вернуть minPriceThreshold при нулевых резервах
	if err != nil {
		d.logger.Error("Error calculating token price, but continuing PnL calculation", zap.Error(err))
		// Можно установить цену в 0 или minPriceThreshold, если расчет не удался
		currentPrice = minPriceThreshold
	}

	// Теоретическая стоимость (tokens * currentPrice)
	theoreticalValue := tokenAmount * currentPrice

	// Оценка продажи с учетом комиссии (но без учета slippage)
	sellEstimate, err := d.CalculateSellValue(ctx, tokenAmount, bondingCurveData)
	if err != nil {
		// Аналогично, не прерываем расчет PnL, но логируем ошибку
		d.logger.Error("Error calculating sell estimate, but continuing PnL calculation", zap.Error(err))
		// Можно установить оценку продажи в 0, если расчет не удался
		sellEstimate = 0
	}

	// Расчет чистого PnL
	netPnL := sellEstimate - initialInvestment

	// Расчет процента PnL
	pnlPercentage := 0.0
	if initialInvestment > 0 {
		// Избегаем деления на ноль
		pnlPercentage = (netPnL / initialInvestment) * 100
	} else if netPnL > 0 {
		// Если начальная инвестиция 0, а PnL положительный, это бесконечный процент
		pnlPercentage = math.Inf(1)

	} // Если и инвестиция 0, и PnL 0 или отрицательный, процент PnL равен 0

	d.logger.Debug("Discrete PnL calculation completed",
		zap.Float64("tokenAmount", tokenAmount),
		zap.Float64("initialInvestment", initialInvestment),
		zap.Float64("current_price", currentPrice),
		zap.Float64("theoretical_value", theoreticalValue),
		zap.Float64("sell_estimate (slippage not included)", sellEstimate),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	return &model.PnLResult{
		SellEstimate:      sellEstimate,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}, nil
}

// GetTokenPrice возвращает текущую цену токена на Pump.fun, используя данные bonding curve.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// проверяем, что запрашиваем именно тот минт,
	// для которого инициализировали DEX
	if d.config.Mint.String() != tokenMint {
		return 0, fmt.Errorf("token %s not configured in this DEX instance", tokenMint)
	}

	// Берём данные кривой из внутреннего TTL‑кэша.
	bcData, _, err := d.getBondingCurveData(ctx)
	if err != nil {
		return 0, err
	}

	// Если кривая «пустая» — значит токен уже уехал на Raydium/PumpSwap
	if bcData.VirtualTokenReserves == 0 || bcData.VirtualSolReserves == 0 {
		d.logger.Warn("Bonding curve may be graduated or not available",
			zap.String("token_mint", tokenMint))
		return 0,
			fmt.Errorf("bonding curve for token %s is graduated or not available", tokenMint)
	}

	return d.CalculateTokenPrice(ctx, bcData)
}

// GetTokenBalance возвращает текущий баланс токена в кошельке пользователя.
// Метод определяет ассоциированный токен-аккаунт для кошелька и запрашивает его баланс.
// Сначала пытается получить баланс с использованием быстрого уровня подтверждения Processed,
// при неудаче переключается на более надежный уровень Confirmed.
func (d *DEX) GetTokenBalance(ctx context.Context, commitment ...rpc.CommitmentType) (uint64, error) {
	// Шаг 1: Вычисление адреса ассоциированного токен-аккаунта (ATA)
	// ATA - стандартизированный адрес для хранения SPL-токенов, уникальный для пары (владелец, минт)
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Шаг 2: Определение уровня подтверждения (commitment level)
	// По умолчанию используем Processed - самый быстрый уровень
	// Можно переопределить через вариативный параметр
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

		// Повторный запрос с более надежным уровнем подтверждения
		result, err = d.client.GetTokenAccountBalance(ctx, userATA, rpc.CommitmentConfirmed)
	}

	// Если ошибка все еще присутствует, возвращаем ее
	if err != nil {
		return 0, fmt.Errorf("failed to get token account balance: %w", err)
	}

	// Шаг 5: Проверка результата на пустоту
	// Убеждаемся, что получены корректные данные
	if result == nil || result.Value.Amount == "" {
		return 0, fmt.Errorf("no token balance found")
	}

	// Шаг 6: Преобразование строкового представления баланса в uint64
	// SPL-токены в Solana представлены как строки для поддержки больших чисел
	balance, err := strconv.ParseUint(result.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token balance: %w", err)
	}

	// Шаг 7: Логирование для отладки
	d.logger.Debug("Got token balance",
		zap.Uint64("balance", balance),
		zap.String("token_mint", d.config.Mint.String()),
		zap.String("user_ata", userATA.String()),
		zap.String("commitment", string(commitmentLevel)))

	// Шаг 8: Возврат полученного баланса
	return balance, nil
}

// ----- Функции для расчёта комиссий -----

// computeCreatorFee вычисляет комиссию создателя токена на основе глобального
// процента creator_fee_basis_points и суммы транзакции
func computeCreatorFee(global *GlobalAccount, bc *BondingCurve, amount uint64, isNewCoin bool) uint64 {
	// Если это не новая монета и creator равен PublicKey{} (нулевой адрес), комиссия не берётся
	isZeroCreator := bc.Creator.Equals(solana.PublicKey{})
	if !isNewCoin && isZeroCreator {
		return 0
	}

	// Иначе рассчитываем комиссию на основе basis points
	// (basis point = 1/100 процента, т.е. 100 бейсис-поинтов = 1%)
	return ceilDiv(amount*global.CreatorFeeBasisPoints, 10_000)
}

// computeTotalFee вычисляет общую комиссию, включая комиссию протокола и creator fee
func computeTotalFee(global *GlobalAccount, bc *BondingCurve, amount uint64, isNewCoin bool) uint64 {
	protocolFee := ceilDiv(amount*global.FeeBasisPoints, 10_000)
	creatorFee := computeCreatorFee(global, bc, amount, isNewCoin)
	return protocolFee + creatorFee
}

// ceilDiv выполняет деление с округлением вверх
func ceilDiv(a, b uint64) uint64 {
	return (a + b - 1) / b
}
