// =============================
// File: internal/dex/pumpfun_adapter.go
// =============================
package dex

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"go.uber.org/zap"
)

// pumpfunDEXAdapter адаптирует Pump.fun к интерфейсу DEX
type pumpfunDEXAdapter struct {
	baseDEXAdapter
	inner *pumpfun.DEX
}

// GetName возвращает название DEX.
func (d *pumpfunDEXAdapter) initPumpFun(_ context.Context, tokenMint string) error {
	// Защищаем от конкурентного доступа к полям адаптера
	d.initMu.Lock()
	defer d.initMu.Unlock()

	// Проверяем, инициализирован ли уже DEX для запрашиваемого токена
	// Это позволяет избежать повторной инициализации и экономит ресурсы
	if d.initDone && d.tokenMint == tokenMint && d.inner != nil {
		return nil // DEX уже инициализирован для этого токена
	}

	// Создаем конфигурацию по умолчанию для DEX
	config := pumpfun.GetDefaultConfig()

	// Настраиваем конфигурацию для конкретного токена
	// Это включает установку адреса токена и связанных с ним параметров
	if err := config.SetupForToken(tokenMint, d.logger); err != nil {
		return fmt.Errorf("failed to setup Pump.fun configuration: %w", err)
	}

	// Создаем новый экземпляр DEX с переданными параметрами
	// Все зависимости (клиент, кошелек, логгер) передаются из адаптера
	var err error
	d.inner, err = pumpfun.NewDEX(d.client, d.wallet, d.logger, config, config.MonitorInterval)
	if err != nil {
		return fmt.Errorf("failed to initialize Pump.fun DEX: %w", err)
	}

	// Сохраняем информацию об инициализированном токене и статусе инициализации
	// для возможности переиспользования DEX в будущих вызовах
	d.tokenMint = tokenMint
	d.initDone = true
	return nil
}

// Execute выполняет операцию на Pump.fun DEX.
func (d *pumpfunDEXAdapter) Execute(ctx context.Context, task *Task) error {
	// Проверка наличия обязательного поля
	if task.TokenMint == "" {
		return fmt.Errorf("token mint address is required for Pump.fun")
	}

	// Ленивая инициализация DEX
	if err := d.initPumpFun(ctx, task.TokenMint); err != nil {
		return err
	}

	switch task.Operation {
	case OperationSnipe:
		// Логирование параметров операции покупки
		d.logger.Info("Executing snipe on Pump.fun",
			zap.String("token_mint", task.TokenMint),
			zap.Float64("sol_amount", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		// Выполнение покупки токена
		return d.inner.ExecuteSnipe(ctx, task.AmountSol, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	case OperationSell:
		// Запрос текущего баланса токенов с подтверждением
		tokenBalance, err := d.inner.GetTokenBalance(ctx, rpc.CommitmentConfirmed)
		if err != nil {
			return fmt.Errorf("failed to get token balance for sell: %w", err)
		}

		if tokenBalance > 0 {
			// Использование фактического баланса для продажи
			d.logger.Info("Executing sell on Pump.fun using actual token balance",
				zap.String("token_mint", task.TokenMint),
				zap.Uint64("token_balance", tokenBalance),
				zap.Float64("slippage_percent", task.SlippagePercent),
				zap.String("priority_fee", task.PriorityFee),
				zap.Uint32("compute_units", task.ComputeUnits))

			return d.inner.ExecuteSell(ctx, tokenBalance, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		} else {
			// Конвертация человеко-читаемого значения в базовые единицы токена
			tokenAmount, err := convertToTokenUnits(ctx, d.inner, task.TokenMint, task.AmountSol, 6)
			if err != nil {
				return err
			}

			// Логирование продажи с конвертированным значением
			d.logger.Info("Executing sell on Pump.fun using converted amount",
				zap.String("token_mint", task.TokenMint),
				zap.Float64("tokens_to_sell", task.AmountSol),
				zap.Uint64("token_amount", tokenAmount),
				zap.Float64("slippage_percent", task.SlippagePercent),
				zap.String("priority_fee", task.PriorityFee),
				zap.Uint32("compute_units", task.ComputeUnits))

			return d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		}

	default:
		// Возвращение ошибки для неподдерживаемых операций
		return fmt.Errorf("operation %s is not supported on Pump.fun", task.Operation)
	}
}

// GetTokenPrice получает текущую цену токена на Pump.fun DEX.
func (d *pumpfunDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.initPumpFun(ctx, tokenMint); err != nil {
		return 0, err
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// GetTokenBalance возвращает текущий баланс токена на аккаунте пользователя.
//
// Метод инициализирует DEX для указанного токена и запрашивает
// баланс соответствующего токена на ассоциированном токен-аккаунте пользователя.
func (d *pumpfunDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	if err := d.initPumpFun(ctx, tokenMint); err != nil {
		return 0, fmt.Errorf("failed to initialize Pump.fun: %w", err)
	}
	// Возвращает количество токенов на балансе в минимальных единицах (без учета десятичных знаков)
	return d.inner.GetTokenBalance(ctx)
}

// SellPercentTokens продает указанный процент имеющихся токенов.
//
// Метод определяет текущий баланс токенов пользователя, вычисляет
// соответствующую указанному проценту долю и выполняет продажу этой
// доли на Pump.fun DEX. Операция выполняется с учетом указанного
// проскальзывания и приоритета транзакции.
func (d *pumpfunDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	if err := d.initPumpFun(ctx, tokenMint); err != nil {
		return err
	}

	return d.inner.SellPercentTokens(ctx, percentToSell, slippagePercent, priorityFeeSol, computeUnits)
}

// CalculateDiscretePnL вычисляет PnL с учетом дискретной структуры Pump.fun.
//
// Метод рассчитывает прибыль и убыток (Profit and Loss) для указанного количества
// токенов с учетом первоначальной инвестиции и особенностей дискретного
// ценообразования на Pump.fun. Расчет учитывает разницу между теоретической
// стоимостью токенов и фактической выручкой при их продаже.
func (d *pumpfunDEXAdapter) CalculateDiscretePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*DiscreteTokenPnL, error) {
	if err := d.initPumpFun(ctx, d.tokenMint); err != nil {
		return nil, fmt.Errorf("failed to initialize Pump.fun: %w", err)
	}

	// Вызываем внутренний метод из пакета pumpfun
	result, err := d.inner.CalculateDiscretePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		return nil, err
	}

	// Конвертируем тип pumpfun.DiscreteTokenPnL в dex.DiscreteTokenPnL
	return &DiscreteTokenPnL{
		CurrentPrice:      result.CurrentPrice,
		TheoreticalValue:  result.TheoreticalValue,
		SellEstimate:      result.SellEstimate,
		InitialInvestment: result.InitialInvestment,
		NetPnL:            result.NetPnL,
		PnLPercentage:     result.PnLPercentage,
	}, nil
}
