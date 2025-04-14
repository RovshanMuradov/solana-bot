// =============================
// File: internal/dex/pumpswap_adapter.go
// =============================
package dex

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
	"go.uber.org/zap"
	"math"
)

// GetName возвращает название DEX.
//
// Метод предоставляет строковый идентификатор для Pump.Swap DEX,
// который используется для логирования и идентификации биржи в системе.
//
// Возвращает:
//   - string: строковое название DEX ("Pump.Swap")
func (d *pumpswapDEXAdapter) GetName() string {
	return "Pump.Swap"
}

// GetTokenPrice получает текущую цену токена на Pump.Swap DEX.
//
// Метод инициализирует DEX для указанного токена и запрашивает
// его текущую рыночную цену на бирже Pump.Swap.
//
// Параметры:
//   - ctx: контекст выполнения
//   - tokenMint: адрес минта токена, для которого запрашивается цена
//
// Возвращает:
//   - float64: цена токена в SOL
//   - error: ошибку, если не удалось получить цену, или nil при успешном выполнении
func (d *pumpswapDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.initPumpSwap(ctx, tokenMint); err != nil {
		return 0, err
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// Execute выполняет операцию на Pump.Swap DEX.
//
// Метод выполняет указанную в задаче операцию (обмен/продажа) на Pump.Swap DEX.
// Перед выполнением операции автоматически инициализирует адаптер для работы с указанным токеном.
// Поддерживает операции OperationSwap (покупка) и OperationSell (продажа).
// При операции обмена (Swap) сумма указывается в SOL и конвертируется в ламппорты.
// При операции продажи определяется точность токена для корректной конвертации.
//
// Параметры:
//   - ctx: контекст выполнения
//   - task: структура с параметрами задачи (тип операции, адрес токена, количество SOL и т.д.)
//
// Возвращает:
//   - error: ошибку, если операция не удалась или не поддерживается, или nil при успешном выполнении
func (d *pumpswapDEXAdapter) Execute(ctx context.Context, task *Task) error {
	if task.TokenMint == "" {
		return fmt.Errorf("token mint address is required for Pump.swap")
	}

	if err := d.initPumpSwap(ctx, task.TokenMint); err != nil {
		return err
	}

	switch task.Operation {
	case OperationSwap:
		d.logger.Info("Executing swap on Pump.swap",
			zap.String("token_mint", task.TokenMint),
			zap.Float64("amount_sol", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		amountLamports := uint64(task.AmountSol * 1e9)

		return d.inner.ExecuteSwap(ctx, pumpswap.SwapParams{
			IsBuy:           true,
			Amount:          amountLamports,
			SlippagePercent: task.SlippagePercent,
			PriorityFeeSol:  task.PriorityFee,
			ComputeUnits:    task.ComputeUnits,
		})

	case OperationSell:
		tokenMintPubkey, err := solana.PublicKeyFromBase58(task.TokenMint)
		if err != nil {
			return fmt.Errorf("invalid token mint address: %w", err)
		}

		precision, err := d.inner.DetermineTokenPrecision(ctx, tokenMintPubkey)
		if err != nil {
			precision = 6
			d.logger.Warn("Could not determine token precision, using default",
				zap.Uint8("default_precision", precision))
		}

		tokenAmount := uint64(task.AmountSol * math.Pow(10, float64(precision)))

		d.logger.Info("Executing sell on Pump.swap",
			zap.String("token_mint", task.TokenMint),
			zap.Float64("tokens_to_sell", task.AmountSol),
			zap.Uint64("token_amount", tokenAmount),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		return d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	default:
		return fmt.Errorf("operation %s is not supported on Pump.swap", task.Operation)
	}
}

// initPumpSwap инициализирует адаптер Pump.Swap DEX при необходимости.
//
// Метод выполняет ленивую инициализацию внутреннего экземпляра DEX для работы с токеном
// по указанному адресу минта. Если DEX уже инициализирован с тем же токеном, метод
// возвращает nil. Безопасен для вызова из нескольких горутин благодаря использованию мьютекса.
// Создает экземпляр менеджера пула и настраивает конфигурацию для указанного токена.
//
// Параметры:
//   - ctx: контекст выполнения (в текущей реализации не используется)
//   - tokenMint: адрес минта токена, для которого инициализируется DEX
//
// Возвращает:
//   - error: ошибку, если инициализация не удалась, или nil при успешной инициализации
func (d *pumpswapDEXAdapter) initPumpSwap(_ context.Context, tokenMint string) error {
	d.initMu.Lock()
	defer d.initMu.Unlock()

	if d.initDone && d.tokenMint == tokenMint && d.inner != nil {
		return nil
	}

	config := pumpswap.GetDefaultConfig()
	if err := config.SetupForToken(tokenMint, d.logger); err != nil {
		return fmt.Errorf("failed to setup Pump.swap configuration: %w", err)
	}

	poolManager := pumpswap.NewPoolManager(d.client, d.logger)

	var err error
	d.inner, err = pumpswap.NewDEX(d.client, d.wallet, d.logger, config, poolManager, config.MonitorInterval)
	if err != nil {
		return fmt.Errorf("failed to initialize Pump.swap DEX: %w", err)
	}

	d.tokenMint = tokenMint
	d.initDone = true
	return nil
}

// GetTokenBalance возвращает текущий баланс токена на аккаунте пользователя.
//
// Метод является заглушкой для совместимости с интерфейсом DEX и в текущей
// реализации не полностью функционален. Логирует вызов с уровнем Debug
// и возвращает ошибку о неполной реализации.
//
// Параметры:
//   - ctx: контекст выполнения
//   - tokenMint: адрес минта токена, для которого запрашивается баланс
//
// Возвращает:
//   - uint64: всегда 0 в текущей реализации
//   - error: ошибку о неполной реализации функциональности
//
// TODO: this is placeholder
func (d *pumpswapDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// В будущем здесь можно реализовать настоящую логику получения баланса
	d.logger.Debug("GetTokenBalance called on PumpSwap (not fully implemented)",
		zap.String("token_mint", tokenMint))

	// Возможно реализовать в будущем, сейчас просто возвращаем ошибку
	return 0, fmt.Errorf("GetTokenBalance not fully implemented for Pump.Swap DEX")
}

// SellPercentTokens продает указанный процент имеющихся токенов.
//
// Метод определяет текущий баланс токенов пользователя, вычисляет
// соответствующую указанному проценту долю и выполняет продажу этой
// доли на Pump.Swap DEX. Операция выполняется с учетом указанного
// проскальзывания и приоритета транзакции. Логирует предупреждение о
// неполной реализации функциональности.
//
// Параметры:
//   - ctx: контекст выполнения
//   - tokenMint: адрес минта токена, который нужно продать
//   - percentToSell: процент токенов для продажи (0-100)
//   - slippagePercent: допустимое проскальзывание цены в процентах
//   - priorityFeeSol: комиссия приоритета в SOL (строковое представление)
//   - computeUnits: количество вычислительных единиц для транзакции
//
// Возвращает:
//   - error: ошибку, если продажа не удалась, или nil при успешном выполнении
//
// TODO: this is placeholder
func (d *pumpswapDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	if err := d.initPumpSwap(ctx, tokenMint); err != nil {
		return err
	}

	d.logger.Warn("SellPercentTokens is not fully implemented for PumpSwap",
		zap.String("token_mint", tokenMint),
		zap.Float64("percent_to_sell", percentToSell))

	// Получаем баланс токена (в настоящее время не реализовано полностью)
	balance, err := d.inner.GetTokenBalance(ctx, tokenMint)
	if err != nil {
		return fmt.Errorf("failed to get token balance: %w", err)
	}

	// Рассчитываем количество токенов для продажи
	tokensToSell := uint64(float64(balance) * percentToSell / 100.0)

	// Выполняем стандартную операцию продажи
	return d.inner.ExecuteSell(ctx, tokensToSell, slippagePercent, priorityFeeSol, computeUnits)
}

// CalculateBondingCurvePnL вычисляет PnL для токена на Pump.Swap DEX.
//
// Метод является упрощенной реализацией для совместимости с интерфейсом.
// Поскольку Pump.Swap не использует дискретную bonding curve, метод
// рассчитывает PnL стандартным способом, где оценка продажи равна
// теоретической стоимости токенов. Получает текущую цену токена и
// на ее основе вычисляет стоимость токенов и показатели прибыли/убытка.
//
// Параметры:
//   - ctx: контекст выполнения
//   - tokenAmount: количество токенов для расчета PnL
//   - initialInvestment: первоначальная инвестиция в SOL
//
// Возвращает:
//   - *DiscreteTokenPnL: структура с информацией о PnL
//   - error: ошибку, если расчет не удался, или nil при успешном выполнении
//
// TODO: this is placeholder. Probably dont even need this func for pumpswap
func (d *pumpswapDEXAdapter) CalculateBondingCurvePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*BondingCurvePnL, error) {
	// PumpSwap не использует дискретную bonding curve, поэтому
	// возвращаем стандартный PnL для совместимости с интерфейсом
	price, err := d.GetTokenPrice(ctx, d.tokenMint)
	if err != nil {
		return nil, err
	}

	theoreticalValue := tokenAmount * price

	return &BondingCurvePnL{
		CurrentPrice:      price,
		TheoreticalValue:  theoreticalValue,
		SellEstimate:      theoreticalValue, // Для не-дискретной кривой оценка равна теоретической стоимости
		InitialInvestment: initialInvestment,
		NetPnL:            theoreticalValue - initialInvestment,
		PnLPercentage:     ((theoreticalValue - initialInvestment) / initialInvestment) * 100,
	}, nil
}
