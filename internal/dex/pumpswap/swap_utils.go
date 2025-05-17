// internal/dex/pumpswap/swap_utils.go
package pumpswap

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"go.uber.org/zap"
	"math"
)

// SellPercentTokens продает указанный процент имеющихся токенов
func (d *DEX) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell, slippage float64, priorityFee string, computeUnits uint32) error {
	if percentToSell <= 0 || percentToSell > 100 {
		return fmt.Errorf("invalid percentage: must be between 0 and 100")
	}

	// Получаем баланс токена
	balance, err := d.GetTokenBalance(ctx, tokenMint)
	if err != nil {
		return fmt.Errorf("failed to get token balance: %w", err)
	}

	if balance == 0 {
		return fmt.Errorf("token balance is zero, nothing to sell")
	}

	// Рассчитываем количество токенов для продажи
	amountToSell := uint64(float64(balance) * percentToSell / 100.0)
	if amountToSell == 0 {
		return fmt.Errorf("amount to sell calculated as zero")
	}

	d.logger.Info("Selling tokens",
		zap.String("token_mint", tokenMint),
		zap.Float64("percent_to_sell", percentToSell),
		zap.Uint64("balance", balance),
		zap.Uint64("amount_to_sell", amountToSell))

	// Выполняем продажу
	return d.ExecuteSell(ctx, amountToSell, slippage, priorityFee, computeUnits)
}

// CalculatePnL вычисляет метрики прибыли и убытка
func (d *DEX) CalculatePnL(ctx context.Context, tokenAmount, initialInvestment float64) (*model.PnLResult, error) {
	if tokenAmount <= 0 {
		return nil, fmt.Errorf("token amount must be positive")
	}

	if initialInvestment <= 0 {
		return nil, fmt.Errorf("initial investment must be positive")
	}

	// Получаем текущую цену токена
	price, err := d.GetTokenPrice(ctx, d.config.BaseMint.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get token price: %w", err)
	}

	// Рассчитываем текущую стоимость токенов
	currentValue, err := d.CalculateSolAmountForTokens(ctx, tokenAmount)
	if err != nil {
		// Если не удалось получить точную оценку, используем приближенный расчет
		currentValue = tokenAmount * price
		d.logger.Warn("Using approximate value calculation",
			zap.Float64("token_amount", tokenAmount),
			zap.Float64("token_price", price))
	}

	// Рассчитываем прибыль/убыток
	profit := currentValue - initialInvestment
	profitPercent := (profit / initialInvestment) * 100.0

	return &model.PnLResult{
		TokenAmount:       tokenAmount,
		TokenPrice:        price,
		InitialInvestment: initialInvestment,
		CurrentValue:      currentValue,
		Profit:            profit,
		ProfitPercent:     profitPercent,
	}, nil
}

// DetermineTokenPrecision определяет точность (decimals) токена
func (d *DEX) DetermineTokenPrecision(ctx context.Context, mint solana.PublicKey) (uint8, error) {
	// Проверяем кэш
	d.cacheMutex.RLock()
	precision, exists := d.precisionCache[mint.String()]
	d.cacheMutex.RUnlock()
	if exists {
		return precision, nil
	}

	// Получаем данные о минте
	accounts, err := d.client.GetMultipleAccounts(ctx, []solana.PublicKey{mint})
	if err != nil {
		return 0, fmt.Errorf("failed to get mint account: %w", err)
	}

	if accounts == nil || len(accounts.Value) == 0 || accounts.Value[0] == nil {
		return 0, fmt.Errorf("mint account not found")
	}

	// Получаем данные минта из аккаунта
	data := accounts.Value[0].Data.GetBinary()
	if len(data) < 46 {
		return 0, fmt.Errorf("invalid mint account data length")
	}

	// Десятичная точность находится в байте 45
	precision = data[45]

	// Кэшируем результат
	d.cacheMutex.Lock()
	d.precisionCache[mint.String()] = precision
	d.cacheMutex.Unlock()

	return precision, nil
}

// GetTokenBalance получает баланс токена
func (d *DEX) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// Парсим адрес минта
	mint, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint address: %w", err)
	}

	// Получаем адрес ATA пользователя
	userPubkey := d.wallet.GetPublicKey()
	ataAddress, _, err := solana.FindAssociatedTokenAddress(userPubkey, mint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive ATA address: %w", err)
	}

	// Получаем баланс токена
	balance, err := d.client.GetTokenAccountBalance(ctx, ataAddress, rpc.CommitmentConfirmed)
	if err != nil {
		// Если аккаунт не найден, значит баланс 0
		if err.Error() == "not found" {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get token balance: %w", err)
	}

	amount := balance.Value.Amount
	if amount == "" {
		return 0, nil
	}

	// Преобразуем строку в uint64
	var amountU64 uint64
	_, err = fmt.Sscanf(amount, "%d", &amountU64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token amount: %w", err)
	}

	return amountU64, nil
}

// GetTokenPrice получает текущую цену токена
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Находим пул
	pool, _, err := d.findAndValidatePool(ctx)
	if err != nil {
		return 0, err
	}

	// Получаем данные пула
	baseReserves := pool.BaseReserves
	quoteReserves := pool.QuoteReserves

	if baseReserves == 0 || quoteReserves == 0 {
		return 0, fmt.Errorf("invalid pool reserves")
	}

	// Определяем precision токена
	mint, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}

	precision, err := d.DetermineTokenPrecision(ctx, mint)
	if err != nil {
		precision = 6 // Используем дефолтное значение, если не удалось получить
		d.logger.Warn("Using default precision", zap.Uint8("precision", precision))
	}

	// Рассчитываем цену
	// Если базовый токен - WSOL, а квотный - наш токен
	if d.config.BaseMint.Equals(solana.SolMint) {
		// Цена WSOL в токенах
		tokenPerSol := float64(quoteReserves) / float64(baseReserves)
		// Конвертируем в цену токена в SOL
		return 1.0 / tokenPerSol, nil
	}

	// Если базовый токен - наш токен, а квотный - WSOL
	solPerToken := float64(quoteReserves) / float64(baseReserves)
	// Учитываем precision
	adjustedPrice := solPerToken / math.Pow(10, float64(9-precision))

	return adjustedPrice, nil
}
