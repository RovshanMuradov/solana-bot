package ui

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

// UpdateSender provides non-blocking UI update sending with statistics
type UpdateSender struct {
	msgChan        chan tea.Msg
	droppedUpdates uint64
	sentUpdates    uint64
	logger         *zap.Logger
	statsInterval  time.Duration
	stopStats      chan struct{}
}

// NewUpdateSender creates a new non-blocking update sender
func NewUpdateSender(msgChan chan tea.Msg, logger *zap.Logger) *UpdateSender {
	us := &UpdateSender{
		msgChan:       msgChan,
		logger:        logger,
		statsInterval: 30 * time.Second,
		stopStats:     make(chan struct{}),
	}

	// Start periodic stats logging
	go us.logStats()

	return us
}

// SendUpdate sends a message to UI without blocking
func (us *UpdateSender) SendUpdate(msg tea.Msg) {
	select {
	case us.msgChan <- msg:
		atomic.AddUint64(&us.sentUpdates, 1)
	default:
		// Log but don't block sniper
		atomic.AddUint64(&us.droppedUpdates, 1)
	}
}

// GetStats returns current statistics
func (us *UpdateSender) GetStats() (sent, dropped uint64) {
	sent = atomic.LoadUint64(&us.sentUpdates)
	dropped = atomic.LoadUint64(&us.droppedUpdates)
	return sent, dropped
}

// logStats periodically logs statistics
func (us *UpdateSender) logStats() {
	ticker := time.NewTicker(us.statsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sent, dropped := us.GetStats()
			if dropped > 0 {
				us.logger.Warn("UI update statistics",
					zap.Uint64("sent", sent),
					zap.Uint64("dropped", dropped),
					zap.Float64("drop_rate", float64(dropped)/float64(sent+dropped)*100))
			}
		case <-us.stopStats:
			return
		}
	}
}

// Close stops the update sender
func (us *UpdateSender) Close() {
	close(us.stopStats)
}

// NonBlockingBus provides a global non-blocking message bus
type NonBlockingBus struct {
	sender *UpdateSender
}

// GlobalBus is the singleton instance
var GlobalBus *NonBlockingBus

// InitBus initializes the global non-blocking bus
func InitBus(msgChan chan tea.Msg, logger *zap.Logger) {
	GlobalBus = &NonBlockingBus{
		sender: NewUpdateSender(msgChan, logger),
	}
}

// Send sends a message without blocking
func (nb *NonBlockingBus) Send(msg tea.Msg) {
	if nb.sender != nil {
		nb.sender.SendUpdate(msg)
	}
}

// GetStats returns bus statistics
func (nb *NonBlockingBus) GetStats() (sent, dropped uint64) {
	if nb.sender != nil {
		return nb.sender.GetStats()
	}
	return 0, 0
}

// Close closes the bus
func (nb *NonBlockingBus) Close() {
	if nb.sender != nil {
		nb.sender.Close()
	}
}
