package screen

import (
	"fmt"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/ui"
)

// Handler methods for real trading events

// handleRealPriceUpdate handles real price updates from monitoring sessions
func (s *MonitorScreen) handleRealPriceUpdate(msg ui.RealPriceUpdateMsg) {
	if msg.TokenMint == "" {
		return // Empty update
	}

	// Find the position for this token
	for i := range s.positions {
		if s.positions[i].TokenMint == msg.TokenMint {
			// Update position with real data
			s.positions[i].CurrentPrice = msg.Update.Current
			s.positions[i].EntryPrice = msg.Update.Initial
			s.positions[i].Amount = msg.Update.Tokens
			s.positions[i].PnLPercent = msg.Update.Percent
			s.positions[i].LastUpdate = time.Now()

			// Calculate PnL in SOL
			initialValue := msg.Update.Initial * msg.Update.Tokens
			currentValue := msg.Update.Current * msg.Update.Tokens
			s.positions[i].PnLSol = currentValue - initialValue

			// Update price history for sparkline
			s.positions[i].PriceHistory = append(s.positions[i].PriceHistory, msg.Update.Current)
			if len(s.positions[i].PriceHistory) > 20 {
				s.positions[i].PriceHistory = s.positions[i].PriceHistory[1:]
			}

			break
		}
	}

	// Update table display
	s.updateTableDisplay()
}

// handlePositionCreated handles position created events
func (s *MonitorScreen) handlePositionCreated(msg ui.PositionCreatedMsg) {
	// Add new position to the list
	position := MonitoredPosition{
		ID:           msg.TaskID,
		TaskName:     fmt.Sprintf("TASK_%d", msg.TaskID),
		TokenMint:    msg.TokenMint,
		TokenSymbol:  msg.TokenSymbol,
		EntryPrice:   msg.EntryPrice,
		CurrentPrice: msg.EntryPrice,
		Amount:       msg.AmountSol,
		PnLPercent:   0,
		PnLSol:       0,
		Volume24h:    0,
		LastUpdate:   time.Now(),
		PriceHistory: []float64{msg.EntryPrice},
		Active:       true,
	}

	s.positions = append(s.positions, position)
	s.updateTableDisplay()
}

// handleMonitoringSessionStarted handles monitoring session started events
func (s *MonitorScreen) handleMonitoringSessionStarted(msg ui.MonitoringSessionStartedMsg) {
	// Update status or add monitoring info
	// This could be used to show session status in UI
}

// handleMonitoringSessionStopped handles monitoring session stopped events
func (s *MonitorScreen) handleMonitoringSessionStopped(msg ui.MonitoringSessionStoppedMsg) {
	// Mark position as inactive or remove it
	for i := range s.positions {
		if s.positions[i].TokenMint == msg.TokenMint {
			s.positions[i].Active = false
			break
		}
	}
	s.updateTableDisplay()
}

// handleSellCompleted handles sell completed events
func (s *MonitorScreen) handleSellCompleted(msg ui.SellCompletedMsg) {
	// Update position after sell or remove if fully sold
	for i := range s.positions {
		if s.positions[i].TokenMint == msg.TokenMint {
			// Update amount after sell
			s.positions[i].Amount -= msg.AmountSold
			if s.positions[i].Amount <= 0 {
				// Remove position if fully sold
				s.positions = append(s.positions[:i], s.positions[i+1:]...)
			} else {
				s.positions[i].LastUpdate = time.Now()
			}
			break
		}
	}
	s.updateTableDisplay()
}
