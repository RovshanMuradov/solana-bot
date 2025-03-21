// =============================
// File: internal/dex/pumpfun/pumpfun.go
// =============================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX is the Pump.fun DEX implementation
type DEX struct {
	client          *solbc.Client
	wallet          *wallet.Wallet
	logger          *zap.Logger
	config          *Config
	priorityManager *types.PriorityManager
}

// NewDEX creates a new instance of the Pump.fun DEX
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, _ string) (*DEX, error) {
	if config.ContractAddress.IsZero() {
		return nil, fmt.Errorf("pump.fun contract address is required")
	}
	if config.Mint.IsZero() {
		return nil, fmt.Errorf("token mint address is required")
	}

	logger.Info("Creating PumpFun DEX",
		zap.String("contract", config.ContractAddress.String()),
		zap.String("token_mint", config.Mint.String()))

	// Create DEX instance
	dex := &DEX{
		client:          client,
		wallet:          w,
		logger:          logger.Named("pumpfun"),
		config:          config,
		priorityManager: types.NewPriorityManager(logger.Named("priority")),
	}

	// Update fee recipient
	fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	globalAccount, err := FetchGlobalAccount(fetchCtx, client, config.Global)
	if err != nil {
		logger.Warn("Failed to fetch global account data, using default fee recipient",
			zap.Error(err))
	} else if globalAccount != nil {
		// Update fee recipient from global account data
		config.FeeRecipient = globalAccount.FeeRecipient
		logger.Info("Updated fee recipient", zap.String("address", config.FeeRecipient.String()))
	}

	return dex, nil
}

// ExecuteSnipe executes a buy operation on the Pump.fun protocol using exact-sol program
func (d *DEX) ExecuteSnipe(ctx context.Context, amountSol float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	d.logger.Info("Starting Pump.fun exact-sol buy operation",
		zap.Float64("amount_sol", amountSol),
		zap.Float64("slippage_percent", slippagePercent),
		zap.String("priority_fee_sol", priorityFeeSol),
		zap.Uint32("compute_units", computeUnits))

	// Create a context with timeout for the entire operation
	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	// Convert SOL amount to lamports
	solAmountLamports := uint64(amountSol * 1_000_000_000)

	// In exact-sol program, we specify exactly how much to spend
	// For now we just use the original amount
	adjustedSolLamports := solAmountLamports

	// If we ever want to adjust for slippage, we can uncomment this:
	/*
		if slippagePercent > 0 {
			adjustedSolLamports = uint64(float64(solAmountLamports) * (1 + slippagePercent/100))
		}
	*/

	d.logger.Info("Using exact SOL amount",
		zap.Uint64("sol_amount_lamports", adjustedSolLamports),
		zap.String("sol_amount", fmt.Sprintf("%.9f SOL", float64(adjustedSolLamports)/1_000_000_000)))

	// Get latest blockhash
	blockhash, err := d.client.GetRecentBlockhash(opCtx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	d.logger.Debug("Got blockhash", zap.String("blockhash", blockhash.String()))

	// Create priority instructions (compute limit and price)
	priorityInstructions, err := d.priorityManager.CreatePriorityInstructions(priorityFeeSol, computeUnits)
	if err != nil {
		return fmt.Errorf("failed to create priority instructions: %w", err)
	}

	// Instruction: Create Associated Token Account Idempotent
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated token account: %w", err)
	}
	ataInstruction := createAssociatedTokenAccountIdempotentInstruction(d.wallet.PublicKey, d.wallet.PublicKey, d.config.Mint)

	// Ensure bonding curve is derived correctly
	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to derive bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve", zap.String("address", bondingCurve.String()))

	// Calculate associated bonding curve ATA
	associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(bondingCurve, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve ATA", zap.String("address", associatedBondingCurve.String()))

	// Create buy-exact-sol instruction
	buyIx := createBuyExactSolInstruction(
		d.config.Global,         // Global account
		d.config.FeeRecipient,   // Fee recipient
		d.config.Mint,           // Token mint
		bondingCurve,            // Bonding curve
		associatedBondingCurve,  // Associated bonding curve ATA
		userATA,                 // User's associated token account
		d.wallet.PublicKey,      // User's wallet
		d.config.EventAuthority, // Event authority
		adjustedSolLamports,     // Exact SOL amount in lamports
	)

	// Assemble all instructions
	var instructions []solana.Instruction

	// Add priority instructions first
	instructions = append(instructions, priorityInstructions...)

	// Add ATA and buy instructions
	instructions = append(instructions, ataInstruction, buyIx)

	// Create transaction
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	if err := d.wallet.SignTransaction(tx); err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Simulate transaction
	simResult, err := d.client.SimulateTransaction(opCtx, tx)
	if err != nil || (simResult != nil && simResult.Err != nil) {
		d.logger.Warn("Transaction simulation failed",
			zap.Error(err),
			zap.Any("sim_error", simResult != nil && simResult.Err != nil))

		// Continue anyway as simulation can sometimes fail for valid transactions
	} else {
		d.logger.Info("Transaction simulation successful",
			zap.Uint64("compute_units", simResult.UnitsConsumed))
	}

	// Send transaction
	txSig, err := d.client.SendTransaction(opCtx, tx)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	d.logger.Info("Buy transaction sent successfully",
		zap.String("signature", txSig.String()))

	// Wait for confirmation
	if err := d.client.WaitForTransactionConfirmation(opCtx, txSig, rpc.CommitmentConfirmed); err != nil {
		d.logger.Warn("Failed to confirm transaction",
			zap.String("signature", txSig.String()),
			zap.Error(err))
		return fmt.Errorf("transaction confirmation failed: %w", err)
	}

	d.logger.Info("Buy transaction confirmed",
		zap.String("signature", txSig.String()))
	return nil
}

// ExecuteSell executes a sell operation on the Pump.fun protocol

func (d *DEX) ExecuteSell(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	d.logger.Info("Starting Pump.fun sell operation",
		zap.Uint64("token_amount", tokenAmount),
		zap.Float64("slippage_percent", slippagePercent),
		zap.String("priority_fee_sol", priorityFeeSol),
		zap.Uint32("compute_units", computeUnits))

	// Create a context with timeout for the entire operation
	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	// Get latest blockhash
	blockhash, err := d.client.GetRecentBlockhash(opCtx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	d.logger.Debug("Got blockhash", zap.String("blockhash", blockhash.String()))

	// Create priority instructions (compute limit and price)
	priorityInstructions, err := d.priorityManager.CreatePriorityInstructions(priorityFeeSol, computeUnits)
	if err != nil {
		return fmt.Errorf("failed to create priority instructions: %w", err)
	}

	// Instruction: Create Associated Token Account Idempotent
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated token account: %w", err)
	}
	ataInstruction := createAssociatedTokenAccountIdempotentInstruction(d.wallet.PublicKey, d.wallet.PublicKey, d.config.Mint)

	// Ensure bonding curve is derived correctly
	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to derive bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve", zap.String("address", bondingCurve.String()))

	// Calculate associated bonding curve ATA
	associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(bondingCurve, d.config.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve ATA", zap.String("address", associatedBondingCurve.String()))

	// Calculate minimum output based on slippage
	// For sell operations we need to estimate the minimum acceptable output
	// Since we don't know the actual token price anymore (as we removed price.go),
	// we'll use a conservative estimate based on the token amount and slippage

	// Внутри ExecuteSell, после получения bondingCurve
	bondingCurveData, err := d.FetchBondingCurveAccount(opCtx, bondingCurve)
	if err != nil {
		return fmt.Errorf("failed to fetch bonding curve data: %w", err)
	}

	// Проверка, не завершена ли bonding curve (примерная проверка)
	if bondingCurveData.VirtualSolReserves < 1000 { // Порог в 0.000001 SOL, можно настроить
		return fmt.Errorf("bonding curve has insufficient SOL reserves, possibly complete")
	}

	// Расчет ожидаемого SOL на основе текущих резервов
	// Простая формула: (tokenAmount * virtual_sol_reserves) / virtual_token_reserves
	// Реальная формула Pump.fun может быть сложнее (например, линейная bonding curve)
	expectedSolValueLamports := (tokenAmount * bondingCurveData.VirtualSolReserves) / bondingCurveData.VirtualTokenReserves

	// Применение проскальзывания
	minSolOutput := uint64(float64(expectedSolValueLamports) * (1.0 - slippagePercent/100.0))

	d.logger.Info("Calculated sell parameters",
		zap.Uint64("token_amount", tokenAmount),
		zap.Uint64("virtual_token_reserves", bondingCurveData.VirtualTokenReserves),
		zap.Uint64("virtual_sol_reserves", bondingCurveData.VirtualSolReserves),
		zap.Uint64("estimated_sol_value_lamports", expectedSolValueLamports),
		zap.Uint64("min_sol_output_lamports", minSolOutput))

	// Instruction #4: sell
	sellIx := createSellInstruction(
		d.config.ContractAddress, // Program ID
		d.config.Global,          // Global account
		d.config.FeeRecipient,    // Fee recipient
		d.config.Mint,            // Token mint
		bondingCurve,             // Bonding curve
		associatedBondingCurve,   // Associated bonding curve ATA
		userATA,                  // User's associated token account
		d.wallet.PublicKey,       // User's wallet
		d.config.EventAuthority,  // Event authority
		tokenAmount,              // Amount of tokens to sell
		minSolOutput,             // Minimum SOL output with slippage
	)

	// Assemble all instructions
	var instructions []solana.Instruction

	// Add priority instructions first
	instructions = append(instructions, priorityInstructions...)

	// Add ATA and sell instructions
	instructions = append(instructions, ataInstruction, sellIx)

	// Create transaction
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	if err := d.wallet.SignTransaction(tx); err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Simulate transaction
	simResult, err := d.client.SimulateTransaction(opCtx, tx)
	if err != nil || (simResult != nil && simResult.Err != nil) {
		d.logger.Warn("Transaction simulation failed",
			zap.Error(err),
			zap.Any("sim_error", simResult != nil && simResult.Err != nil))

		// Continue anyway as simulation can sometimes fail for valid transactions
	} else {
		d.logger.Info("Transaction simulation successful",
			zap.Uint64("compute_units", simResult.UnitsConsumed))
	}

	// Замените обработку ошибки после SendTransaction
	txSig, err := d.client.SendTransaction(opCtx, tx)
	if err != nil {
		// Проверяем на ошибку BondingCurveComplete (код 0x1775 или 6005)
		if strings.Contains(err.Error(), "BondingCurveComplete") ||
			strings.Contains(err.Error(), "0x1775") ||
			strings.Contains(err.Error(), "6005") {
			d.logger.Error("Невозможно продать токен через Pump.fun",
				zap.String("token_mint", d.config.Mint.String()),
				zap.String("reason", "Токен перенесен на Raydium"))
			return fmt.Errorf("токен %s перенесен на Raydium и не может быть продан через Pump.fun",
				d.config.Mint.String())
		}
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	d.logger.Info("Sell transaction sent successfully",
		zap.String("signature", txSig.String()))

	// Wait for confirmation
	if err := d.client.WaitForTransactionConfirmation(opCtx, txSig, rpc.CommitmentConfirmed); err != nil {
		d.logger.Warn("Failed to confirm transaction",
			zap.String("signature", txSig.String()),
			zap.Error(err))
		return fmt.Errorf("transaction confirmation failed: %w", err)
	}

	d.logger.Info("Sell transaction confirmed",
		zap.String("signature", txSig.String()))
	return nil
}

func (d *DEX) FetchBondingCurveAccount(ctx context.Context, bondingCurve solana.PublicKey) (*BondingCurve, error) {
	// Получаем информацию об аккаунте
	accountInfo, err := d.client.GetAccountInfo(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve account: %w", err)
	}

	// Проверяем, существует ли аккаунт
	if accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}

	// Извлекаем данные из accountInfo.Value.Data
	data := accountInfo.Value.Data.GetBinary()
	if len(data) < 16 {
		return nil, fmt.Errorf("invalid bonding curve data: insufficient length")
	}

	// Парсим первые 16 байт как virtual_token_reserves и virtual_sol_reserves
	virtualTokenReserves := binary.LittleEndian.Uint64(data[0:8])
	virtualSolReserves := binary.LittleEndian.Uint64(data[8:16])

	return &BondingCurve{
		VirtualTokenReserves: virtualTokenReserves,
		VirtualSolReserves:   virtualSolReserves,
	}, nil
}

// GetTokenPrice возвращает текущую цену токена на основе bonding curve
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Если это тот же токен, который уже настроен
	if d.config.Mint.String() == tokenMint {
		// Получить bonding curve
		bondingCurve, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
			d.config.ContractAddress,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to derive bonding curve: %w", err)
		}

		// Получить данные bonding curve
		bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch bonding curve data: %w", err)
		}

		// Расчет цены как соотношение SOL/токен (используется формула Pump.fun)
		// Для 1 токена цена будет:
		price := float64(bondingCurveData.VirtualSolReserves) / float64(bondingCurveData.VirtualTokenReserves)

		// Форматировать до 9 знаков после запятой (как SOL)
		price = math.Floor(price*1e9) / 1e9

		return price, nil
	}

	return 0, fmt.Errorf("token %s not configured in this DEX instance", tokenMint)
}
