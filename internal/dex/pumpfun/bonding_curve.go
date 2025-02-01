// internal/dex/pumpfun/bonding_curve.go
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

// BondingCurveInfo содержит данные о состоянии bonding curve.
type BondingCurveInfo struct {
	Progress    float64   // Прогресс bonding curve (в %)
	TotalSOL    float64   // Общее количество SOL (в SOL)
	MarketCap   float64   // Рыночная капитализация токена (в SOL)
	LastUpdated time.Time // Время последнего обновления
}

// BondingCurveMonitor осуществляет периодический опрос состояния bonding curve.
type BondingCurveMonitor struct {
	client          *solbc.Client
	logger          *zap.Logger
	monitorInterval time.Duration
	currentState    *BondingCurveInfo
	// Можно добавить канал уведомлений о достижении порогового значения
	notify chan *BondingCurveInfo
}

// NewBondingCurveMonitor создаёт новый экземпляр мониторинга.
func NewBondingCurveMonitor(client *solbc.Client, logger *zap.Logger, interval time.Duration) *BondingCurveMonitor {
	return &BondingCurveMonitor{
		client:          client,
		logger:          logger.Named("bonding-curve-monitor"),
		monitorInterval: interval,
		notify:          make(chan *BondingCurveInfo, 1),
	}
}

// Start запускает цикл мониторинга bonding curve до отмены контекста.
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
			m.logger.Debug("Bonding curve state updated",
				zap.Float64("progress", state.Progress),
				zap.Float64("total_sol", state.TotalSOL),
				zap.Float64("market_cap", state.MarketCap))
			// Если требуется уведомление, можно отправить новое состояние в канал
			select {
			case m.notify <- state:
			default:
			}
		}
	}
}

// GetCurrentState возвращает последнее полученное состояние bonding curve.
func (m *BondingCurveMonitor) GetCurrentState() (*BondingCurveInfo, error) {
	if m.currentState == nil {
		return nil, fmt.Errorf("bonding curve state not available yet")
	}
	return m.currentState, nil
}

// Subscribe возвращает канал уведомлений о новых данных bonding curve.
func (m *BondingCurveMonitor) Subscribe() <-chan *BondingCurveInfo {
	return m.notify
}

// queryBondingCurve осуществляет запрос данных из аккаунта bonding curve.
func (m *BondingCurveMonitor) queryBondingCurve(ctx context.Context) (*BondingCurveInfo, error) {
	// Здесь предполагается, что адрес bonding curve берётся из конфигурации (например, через PumpfunConfig)
	// Замените "BONDING_CURVE_ADDRESS" на реальный адрес или передавайте его через конфигурацию.
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
	if len(data) < 24 {
		return nil, fmt.Errorf("insufficient bonding curve data length: %d", len(data))
	}
	// Пример: первые 8 байт – TotalSOL (в лампортах), следующие 8 – Progress (в сотых процентах), следующие 8 – MarketCap (в лампортах)
	totalSOLLamports := binary.LittleEndian.Uint64(data[0:8])
	progressRaw := binary.LittleEndian.Uint64(data[8:16])
	marketCapLamports := binary.LittleEndian.Uint64(data[16:24])

	// Преобразуем lamports в SOL (1 SOL = 1e9 lamports)
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
