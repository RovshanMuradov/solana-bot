// ======================================
// File: internal/dex/pumpfun/pumpfun.go
// ======================================
package pumpfun

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX is the Pump.fun DEX implementation.
type DEX struct {
	client        *solbc.Client
	wallet        *wallet.Wallet // Добавляем ссылку на кошелёк
	logger        *zap.Logger
	config        *Config
	monitor       *BondingCurveMonitor
	events        *Monitor
	graduated     bool
	raydiumClient *raydium.Client
}

// NewDEX создаёт новый экземпляр DEX, теперь принимающий кошелёк в параметрах.
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, monitorInterval string) (*DEX, error) {
	interval, err := time.ParseDuration(monitorInterval)
	if err != nil {
		return nil, err
	}
	return &DEX{
		client:        client,
		wallet:        w,
		logger:        logger.Named("pumpfun"),
		config:        config,
		monitor:       NewBondingCurveMonitor(client, logger, interval),
		events:        NewPumpfunMonitor(logger, interval),
		raydiumClient: nil, // Устанавливайте отдельно, если требуется
	}, nil
}

// CreateAndSendTransaction – вспомогательная функция, создающая транзакцию, подписывающая её с помощью кошелька и отправляющая через клиента.
func CreateAndSendTransaction(ctx context.Context, client *solbc.Client, w *wallet.Wallet, instructions []solana.Instruction) (solana.Signature, error) {
	blockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(w.PublicKey))
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}
	if err := w.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}
	sig, err := client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}
	return sig, nil
}

func (d *DEX) ExecuteSnipe(ctx context.Context, amount, maxSolCost uint64) error {
	if d.graduated {
		d.logger.Info("Token has graduated. Redirecting snipe to Raydium.")
		if d.raydiumClient == nil {
			return fmt.Errorf("no Raydium client set for graduated token")
		}
		snipeParams := &raydium.SnipeParams{
			TokenMint:           d.config.Mint,
			SourceMint:          solana.MustPublicKeyFromBase58("SOURCE_MINT"),
			AmmAuthority:        solana.MustPublicKeyFromBase58("AMM_AUTHORITY"),
			BaseVault:           solana.MustPublicKeyFromBase58("BASE_VAULT"),
			QuoteVault:          solana.MustPublicKeyFromBase58("QUOTE_VAULT"),
			UserPublicKey:       d.wallet.PublicKey,
			PrivateKey:          &d.wallet.PrivateKey,
			UserSourceATA:       solana.MustPublicKeyFromBase58("USER_SOURCE_ATA"),
			UserDestATA:         solana.MustPublicKeyFromBase58("USER_DEST_ATA"),
			AmountInLamports:    amount,
			MinOutLamports:      maxSolCost,
			PriorityFeeLamports: 0,
		}
		_, err := d.raydiumClient.Snipe(ctx, snipeParams)
		return err
	}

	d.logger.Info("Executing Pump.fun snipe")
	buyAccounts := BuyInstructionAccounts{
		Global:                 d.config.Global,
		FeeRecipient:           d.config.FeeRecipient,
		Mint:                   d.config.Mint,
		BondingCurve:           d.config.BondingCurve,
		AssociatedBondingCurve: d.config.AssociatedBondingCurve,
		EventAuthority:         d.config.EventAuthority,
		Program:                d.config.ContractAddress,
	}
	userWallet := d.wallet
	buyIx, err := BuildBuyTokenInstruction(buyAccounts, userWallet, amount, maxSolCost)
	if err != nil {
		return fmt.Errorf("failed to build buy instruction: %w", err)
	}

	txSig, err := CreateAndSendTransaction(ctx, d.client, d.wallet, []solana.Instruction{buyIx})
	if err != nil {
		return fmt.Errorf("failed to send Pump.fun snipe transaction: %w", err)
	}
	d.logger.Info("Pump.fun snipe transaction sent", zap.String("tx", txSig.String()))

	go d.monitor.Start(ctx)
	go d.events.Start(ctx)

	return nil
}

func (d *DEX) ExecuteSell(ctx context.Context, amount, minSolOutput uint64) error {
	if !d.config.AllowSellBeforeFull {
		return fmt.Errorf("selling not allowed before 100%% bonding curve")
	}
	d.logger.Info("Executing Pump.fun sell", zap.Uint64("amount", amount))
	userWallet := d.wallet

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
		return fmt.Errorf("failed to build sell token instruction: %w", err)
	}

	txSig, err := CreateAndSendTransaction(ctx, d.client, d.wallet, []solana.Instruction{sellIx})
	if err != nil {
		return fmt.Errorf("failed to send sell transaction: %w", err)
	}
	d.logger.Info("Pump.fun sell transaction sent", zap.String("tx", txSig.String()))
	return nil
}

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
				ExtraData:           []byte{},
			}
			// Передаём кошелёк d.wallet в функцию GraduateToken
			sig, err := GraduateToken(ctx, d.client, d.wallet, d.logger, params, d.config.ContractAddress)
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
