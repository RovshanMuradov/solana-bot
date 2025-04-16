// internal/dex/pumpfun/token_calc.go
package pumpfun

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"math"
	"strconv"

	"go.uber.org/zap"
)

const (
	// Стандартные десятичные знаки для SOL и токенов Pump.fun
	solDecimals   = 9
	tokenDecimals = 6
	// Примерная комиссия за транзакцию на Pump.fun (1%)
	pumpFeePercentage = 0.01
	// Минимальная цена для предотвращения деления на ноль или слишком малых значений
	minPriceThreshold = 1e-18 // Очень маленькое значение, близкое к нулю
)

// BondingCurvePnL содержит информацию о прибыли/убытке (PnL) токена
type BondingCurvePnL struct {
	CurrentPrice      float64 // Текущая цена токена (SOL за токен)
	TheoreticalValue  float64 // Теоретическая стоимость текущей позиции: токены * CurrentPrice
	SellEstimate      float64 // Приблизительная выручка при продаже (цена * кол-во * (1 - комиссия))
	InitialInvestment float64 // Первоначальные вложения в SOL
	NetPnL            float64 // Чистая прибыль/убыток: SellEstimate - InitialInvestment
	PnLPercentage     float64 // Процент PnL от начальных вложений
}

// CalculateTokenPrice рассчитывает текущую спотовую цену токена на основе виртуальных резервов bonding curve.
// Формула: Price = (VirtualSolReserves / 10^solDecimals) / (VirtualTokenReserves / 10^tokenDecimals)
// Эта формула является аппроксимацией и может отличаться от точной математической модели кривой Pump.fun.
func (d *DEX) CalculateTokenPrice(ctx context.Context, bondingCurveData *BondingCurve) (float64, error) {
	if bondingCurveData == nil {
		return 0, fmt.Errorf("bonding curve data is nil")
	}

	// Проверка на нулевые резервы токенов для избежания деления на ноль
	if bondingCurveData.VirtualTokenReserves == 0 {
		d.logger.Warn("Virtual token reserves are zero, cannot calculate price accurately. Returning minimum threshold.")
		// Возвращаем очень маленькую цену или ноль, в зависимости от требуемой логики
		return minPriceThreshold, nil // Или можно вернуть ошибку: fmt.Errorf("virtual token reserves are zero")
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
// исходя из текущей цены и с учётом комиссии.
// ВАЖНО: Эта функция НЕ учитывает проскальзывание (slippage) - влияние самой продажи на цену.
// Она просто умножает количество токенов на текущую цену и вычитает комиссию.
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

	// Применяем комиссию
	sellEstimate := baseValue * (1.0 - pumpFeePercentage)

	d.logger.Debug("Sell estimate calculation (slippage NOT included)",
		zap.Float64("tokenAmount", tokenAmount),
		zap.Float64("currentPrice", currentPrice),
		zap.Float64("baseValue", baseValue),
		zap.Float64("feePercentage", pumpFeePercentage),
		zap.Float64("sellEstimate", sellEstimate))

	// Дополнительное логирование, если оценка продажи равна нулю
	if sellEstimate <= 0 {
		d.logger.Warn("Sell estimate is zero or negative",
			zap.Float64("tokenAmount", tokenAmount),
			zap.Float64("currentPrice", currentPrice))
	}

	return sellEstimate, nil
}

// CalculateBondingCurvePnL вычисляет PnL (прибыль/убыток) на основе модели bonding curve.
// Расчет учитывает виртуальные резервы токена и SOL, применяет комиссию протокола,
// но НЕ учитывает проскальзывание (slippage) при больших объемах продажи.
func (d *DEX) CalculateBondingCurvePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*BondingCurvePnL, error) {
	// Получаем данные bonding curve через функции deriveBondingCurveAccounts и FetchBondingCurveAccount
	bondingCurveAddr, _, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to derive bonding curve accounts: %w", err)
	}

	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurveAddr)
	if err != nil {
		// Попытка обработать случай, когда аккаунт еще не создан (например, токен только что запущен)
		// В этом случае резервы равны нулю, и цена должна быть минимальной.
		d.logger.Warn("Failed to fetch bonding curve data, assuming zero reserves", zap.Error(err))
		bondingCurveData = &BondingCurve{VirtualSolReserves: 0, VirtualTokenReserves: 0} // Используем нулевые резервы
	}

	// Расчёт текущей цены
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

	return &BondingCurvePnL{
		CurrentPrice:      currentPrice,
		TheoreticalValue:  theoreticalValue,
		SellEstimate:      sellEstimate,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}, nil
}

// GetTokenPrice возвращает текущую цену токена на Pump.fun, используя данные bonding curve.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Проверка соответствия запрашиваемого токена настроенному в DEX
	if d.config.Mint.String() != tokenMint {
		return 0, fmt.Errorf("token %s not configured in this DEX instance", tokenMint)
	}

	// Вычисление адреса bonding curve для токена
	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to derive bonding curve: %w", err)
	}

	// Получение данных аккаунта bonding curve с менее строгим уровнем подтверждения
	// для более быстрого доступа к новым токенам
	accountInfo, err := d.client.GetAccountInfo(ctx, bondingCurve)

	// Если аккаунт не найден, возможно, это очень новый токен
	if err != nil || accountInfo == nil || accountInfo.Value == nil {
		// Для новых токенов возвращаем минимальную цену
		d.logger.Warn("Bonding curve account not found or error, assuming new token with minimal price",
			zap.String("token_mint", tokenMint),
			zap.Error(err))
		return 0.000000001, nil // Минимальная цена для новых токенов
	}

	// Парсим данные bonding curve
	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		d.logger.Warn("Failed to parse bonding curve data, assuming new token",
			zap.String("token_mint", tokenMint),
			zap.Error(err))
		return 0.000000001, nil
	}

	// Проверка на нулевые резервы (характерно для очень новых токенов)
	if bondingCurveData.VirtualTokenReserves == 0 || bondingCurveData.VirtualSolReserves == 0 {
		d.logger.Warn("Bonding curve has zero reserves, assuming new token",
			zap.String("token_mint", tokenMint))
		return 0.000000001, nil
	}

	// Используем общую функцию для расчета цены токена
	return d.CalculateTokenPrice(ctx, bondingCurveData)
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
	if len(commitment) > 0 && commitment[0] != "" {
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
