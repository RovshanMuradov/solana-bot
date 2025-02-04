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

// BondingCurveInfo holds bonding curve state data.
type BondingCurveInfo struct {
	Progress    float64
	TotalSOL    float64
	MarketCap   float64
	LastUpdated time.Time
}

// BondingCurveMonitor periodically polls the bonding curve state.
type BondingCurveMonitor struct {
	client          *solbc.Client
	logger          *zap.Logger
	monitorInterval time.Duration
	currentState    *BondingCurveInfo
	notify          chan *BondingCurveInfo
}

// NewBondingCurveMonitor creates a new instance of bonding curve monitoring.
func NewBondingCurveMonitor(client *solbc.Client, logger *zap.Logger, interval time.Duration) *BondingCurveMonitor {
	return &BondingCurveMonitor{
		client:          client,
		logger:          logger.Named("bonding-curve-monitor"),
		monitorInterval: interval,
		notify:          make(chan *BondingCurveInfo, 1),
	}
}

func (m *BondingCurveMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.monitorInterval)
	defer ticker.Stop()
	m.logger.Info("Bonding curve monitor started", zap.Duration("interval", m.monitorInterval))
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Bonding curve monitor stopped")
			return
		case <-ticker.C:
			state, err := m.queryBondingCurve(ctx)
			if err != nil {
				m.logger.Error("Failed to query bonding curve state", zap.Error(err))
				continue
			}
			m.currentState = state
			m.logger.Debug("Bonding curve updated",
				zap.Float64("progress", state.Progress),
				zap.Float64("total_sol", state.TotalSOL),
				zap.Float64("market_cap", state.MarketCap))
			select {
			case m.notify <- state:
			default:
			}
		}
	}
}

func (m *BondingCurveMonitor) GetCurrentState() (*BondingCurveInfo, error) {
	if m.currentState == nil {
		return nil, fmt.Errorf("bonding curve state not available yet")
	}
	return m.currentState, nil
}

func (m *BondingCurveMonitor) Subscribe() <-chan *BondingCurveInfo {
	return m.notify
}

func (m *BondingCurveMonitor) queryBondingCurve(ctx context.Context) (*BondingCurveInfo, error) {
	bondingCurveAddr, err := solana.PublicKeyFromBase58("BONDING_CURVE_ADDRESS")
	if err != nil {
		return nil, fmt.Errorf("invalid bonding curve address: %w", err)
	}
	accountInfo, err := m.client.GetAccountInfo(ctx, bondingCurveAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve account info: %w", err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}
	data := accountInfo.Value.Data.GetBinary()
	m.logger.Debug("Raw bonding curve data", zap.ByteString("data", data))

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
