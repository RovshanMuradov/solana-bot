// internal/dex/pumpfun/pumpfun.go
package pumpfun

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// DEX реализует интерфейс для работы с Pump.fun.
type DEX struct {
	client        *solbc.Client // Клиент для отправки транзакций
	logger        *zap.Logger
	config        *PumpfunConfig // Конфигурация Pump.fun
	monitor       *BondingCurveMonitor
	events        *PumpfunMonitor
	graduated     bool            // Флаг, показывающий, что токен перешёл в режим Raydium
	raydiumClient *raydium.Client // Клиент для работы с Raydium
}

// NewDEX создает новый экземпляр Pump.fun DEX.
func NewDEX(client *solbc.Client, logger *zap.Logger, config *PumpfunConfig, monitorIntervalDuration string) (*DEX, error) {
	interval, err := parseDuration(monitorIntervalDuration)
	if err != nil {
		return nil, err
	}
	return &DEX{
		client:  client,
		logger:  logger.Named("pumpfun"),
		config:  config,
		monitor: NewBondingCurveMonitor(client, logger, interval),
		events:  NewPumpfunMonitor(logger, interval),
	}, nil
}

// parseDuration преобразует строковое значение в time.Duration.
func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// ExecuteSnipe выполняет покупку токена через Pump.fun.
// Если токен уже graduated, перенаправляет снайпинг на Raydium.
func (d *DEX) ExecuteSnipe(ctx context.Context, amount, maxSolCost uint64) error {
	// Если токен уже перешёл в режим Raydium, перенаправляем операцию.
	if d.graduated {
		d.logger.Info("Token has graduated. Redirecting snipe operation to Raydium.")
		// Здесь создаём параметры для Raydium Snipe.
		// Заполните необходимые поля SnipeParams согласно вашей логике.
		snipeParams := &raydium.SnipeParams{
			TokenMint: d.config.Mint,
			// Остальные параметры, например:
			SourceMint:    solana.MustPublicKeyFromBase58("SOURCE_MINT"), // например, USDC или wSOL
			AmmAuthority:  solana.MustPublicKeyFromBase58("AMM_AUTHORITY"),
			BaseVault:     solana.MustPublicKeyFromBase58("BASE_VAULT"),
			QuoteVault:    solana.MustPublicKeyFromBase58("QUOTE_VAULT"),
			UserPublicKey: d.client.GetWallet().PublicKey,
			PrivateKey:    &d.client.GetWallet().PrivateKey,
			// ATA пользователя и суммы:
			UserSourceATA:       solana.MustPublicKeyFromBase58("USER_SOURCE_ATA"),
			UserDestATA:         solana.MustPublicKeyFromBase58("USER_DEST_ATA"),
			AmountInLamports:    amount,
			MinOutLamports:      maxSolCost, // или другой параметр минимального выхода
			PriorityFeeLamports: 0,
		}
		_, err := d.raydiumClient.Snipe(ctx, snipeParams)
		return err
	}

	d.logger.Info("Executing Pump.fun snipe")
	// Собираем структуру аккаунтов для инструкции покупки.
	buyAccounts := BuyInstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
	}

	// Получаем пользовательский кошелёк.
	userWallet := d.client.GetWallet()

	// Строим инструкцию покупки токена.
	buyIx, err := BuildBuyTokenInstruction(buyAccounts, userWallet, amount, maxSolCost)
	if err != nil {
		return fmt.Errorf("failed to build buy token instruction: %w", err)
	}

	// Отправляем транзакцию.
	txSig, err := d.client.CreateAndSendTransaction(ctx, []solana.Instruction{buyIx})
	if err != nil {
		return fmt.Errorf("failed to send pumpfun snipe transaction: %w", err)
	}
	d.logger.Info("Pump.fun snipe transaction sent", zap.String("tx", txSig.String()))

	// Запускаем мониторинг bonding curve.
	go d.monitor.Start(ctx)
	go d.events.Start(ctx)

	return nil
}

// ExecuteSell выполняет продажу токена через Pump.fun до достижения 100% bonding curve.
func (d *DEX) ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error {
	if !d.config.AllowSellBeforeFull {
		return fmt.Errorf("продажа не разрешена до достижения 100%% bonding curve")
	}
	d.logger.Info("Executing Pump.fun sell", zap.Uint64("amount", amount))

	// Получаем кошелек пользователя (предполагается, что клиент предоставляет этот метод).
	userWallet := d.client.GetWallet()

	// Собираем аккаунты для инструкции продажи.
	sellAccounts := SellInstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
	}

	sellIx, err := BuildSellTokenInstruction(sellAccounts, userWallet, amount, minSolOutput)
	if err != nil {
		return fmt.Errorf("не удалось сформировать инструкцию продажи токена: %w", err)
	}

	txSig, err := d.client.CreateAndSendTransaction(ctx, []solana.Instruction{sellIx})
	if err != nil {
		return fmt.Errorf("не удалось отправить транзакцию продажи: %w", err)
	}
	d.logger.Info("Pumpfun sell transaction sent", zap.String("tx", txSig.String()))
	return nil
}

// CheckForGraduation проверяет, достигла ли bonding curve порога graduation.
// Если достигнуто и токен ещё не переведён в режим Raydium, вызывается GraduateToken.
func (d *DEX) CheckForGraduation(ctx context.Context) (bool, error) {
	state, err := d.monitor.GetCurrentState()
	if err != nil {
		return false, err
	}
	d.logger.Debug("Bonding curve progress", zap.Float64("progress", state.Progress))
	if state.Progress >= d.config.GraduationThreshold {
		if !d.graduated {
			params := &GraduateParams{
				TokenMint:           d.config.Mint,
				BondingCurveAccount: d.config.BondingCurve,
				ExtraData:           []byte{}, // при необходимости можно добавить дополнительные данные
			}
			sig, err := GraduateToken(ctx, d.client, d.logger, params, d.config.ContractAddress)
			if err != nil {
				d.logger.Error("Graduation transaction failed", zap.Error(err))
			} else {
				d.logger.Info("Graduation transaction sent", zap.String("signature", sig.String()))
				d.graduated = true
			}
		}
		return true, nil
	}
	return false, nil
}
