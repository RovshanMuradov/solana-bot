// ==============================================
// File: internal/dex/pumpfun/bonding_curve.go
// ==============================================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// QueryBondingCurveState queries the bonding curve account for its current state
func QueryBondingCurveState(ctx context.Context, client *solbc.Client, bondingCurveAddr solana.PublicKey, _ *zap.Logger) (*BondingCurveInfo, error) {
	accountInfo, err := client.GetAccountInfo(ctx, bondingCurveAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve account info: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found at %s", bondingCurveAddr.String())
	}

	data := accountInfo.Value.Data.GetBinary()

	if len(data) < 24 {
		return nil, fmt.Errorf("insufficient bonding curve data length: %d", len(data))
	}

	totalSOLLamports := binary.LittleEndian.Uint64(data[0:8])
	progressRaw := binary.LittleEndian.Uint64(data[8:16])
	marketCapLamports := binary.LittleEndian.Uint64(data[16:24])

	totalSOL := float64(totalSOLLamports) / 1e9
	progress := float64(progressRaw) / 100.0
	marketCap := float64(marketCapLamports) / 1e9

	return &BondingCurveInfo{
		Progress:    progress,
		TotalSOL:    totalSOL,
		MarketCap:   marketCap,
		LastUpdated: time.Now(),
	}, nil
}
