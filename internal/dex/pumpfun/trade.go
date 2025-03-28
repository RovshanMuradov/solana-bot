// =============================
// File: internal/dex/pumpfun/trade.go
// =============================
package pumpfun

import (
	"context"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// prepareBuyTransaction подготавливает транзакцию покупки
func (d *DEX) prepareBuyTransaction(ctx context.Context, solAmountLamports uint64, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, error) {
	// Prepare base instructions (priority and ATA)
	baseInstructions, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// Get bonding curve accounts
	bondingCurve, associatedBondingCurve, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, err
	}

	// Create buy instruction
	buyIx := createBuyExactSolInstruction(
		d.config.Global,         // Global account
		d.config.FeeRecipient,   // Fee recipient
		d.config.Mint,           // Token mint
		bondingCurve,            // Bonding curve
		associatedBondingCurve,  // Associated bonding curve ATA
		userATA,                 // User's associated token account
		d.wallet.PublicKey,      // User's wallet
		d.config.EventAuthority, // Event authority
		solAmountLamports,       // Exact SOL amount in lamports
	)

	// Add buy instruction to base instructions
	instructions := append(baseInstructions, buyIx)
	return instructions, nil
}

// prepareSellTransaction подготавливает транзакцию продажи
func (d *DEX) prepareSellTransaction(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, error) {
	// Prepare base instructions (priority and ATA)
	baseInstructions, userATA, err := d.prepareBaseInstructions(ctx, priorityFeeSol, computeUnits)
	if err != nil {
		return nil, err
	}

	// Get bonding curve accounts
	bondingCurve, associatedBondingCurve, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, err
	}

	// Get bonding curve data
	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bonding curve data: %w", err)
	}

	// Check if bonding curve is complete
	if bondingCurveData.VirtualSolReserves < 1000 {
		return nil, fmt.Errorf("bonding curve has insufficient SOL reserves, possibly complete")
	}

	// Calculate minimum output with slippage
	minSolOutput := d.calculateMinSolOutput(tokenAmount, bondingCurveData, slippagePercent)

	d.logger.Info("Calculated sell parameters",
		zap.Uint64("token_amount", tokenAmount),
		zap.Uint64("virtual_token_reserves", bondingCurveData.VirtualTokenReserves),
		zap.Uint64("virtual_sol_reserves", bondingCurveData.VirtualSolReserves),
		zap.Uint64("min_sol_output_lamports", minSolOutput))

	// Create sell instruction
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

	// Add sell instruction to base instructions
	instructions := append(baseInstructions, sellIx)
	return instructions, nil
}

// calculateMinSolOutput вычисляет минимальный выход SOL с учетом проскальзывания
func (d *DEX) calculateMinSolOutput(tokenAmount uint64, bondingCurveData *BondingCurve, slippagePercent float64) uint64 {
	expectedSolValueLamports := (tokenAmount * bondingCurveData.VirtualSolReserves) / bondingCurveData.VirtualTokenReserves
	return uint64(float64(expectedSolValueLamports) * (1.0 - slippagePercent/100.0))
}

// handleSellError обрабатывает ошибки при продаже
func (d *DEX) handleSellError(err error) error {
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
