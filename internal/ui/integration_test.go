package ui_test

import (
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/state"
	"go.uber.org/zap"
)

// simulatedSniperBot represents a mock sniper bot that continues trading
type simulatedSniperBot struct {
	tradesExecuted int32
	pricesUpdated  int32
	isRunning      int32
	logger         *zap.Logger
}

func newSimulatedSniperBot(logger *zap.Logger) *simulatedSniperBot {
	return &simulatedSniperBot{
		logger:    logger,
		isRunning: 1,
	}
}

func (s *simulatedSniperBot) start() {
	// Simulate trading operations
	go func() {
		for atomic.LoadInt32(&s.isRunning) == 1 {
			// Execute trade
			atomic.AddInt32(&s.tradesExecuted, 1)
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Simulate price updates
	go func() {
		for atomic.LoadInt32(&s.isRunning) == 1 {
			// Update price
			atomic.AddInt32(&s.pricesUpdated, 1)

			// Send to UI if available
			if ui.GlobalBus != nil {
				update := monitor.PriceUpdate{
					Current: float64(atomic.LoadInt32(&s.pricesUpdated)),
					Initial: 100.0,
					Percent: float64(atomic.LoadInt32(&s.pricesUpdated)) / 100.0,
				}
				ui.GlobalBus.Send(ui.PriceUpdateMsg{Update: update})
			}

			time.Sleep(20 * time.Millisecond)
		}
	}()
}

func (s *simulatedSniperBot) stop() {
	atomic.StoreInt32(&s.isRunning, 0)
}

func (s *simulatedSniperBot) getStats() (trades, prices int32) {
	return atomic.LoadInt32(&s.tradesExecuted), atomic.LoadInt32(&s.pricesUpdated)
}

// TestUIIsolationIntegration tests that sniper continues during UI crashes
func TestUIIsolationIntegration(t *testing.T) {
	logger := zap.NewNop()

	// Initialize UI infrastructure
	msgChan := make(chan tea.Msg, 1024)
	ui.InitBus(msgChan, logger)
	state.InitCache(logger)

	// Start simulated sniper bot
	sniperBot := newSimulatedSniperBot(logger)
	sniperBot.start()
	defer sniperBot.stop()

	// Create UI that will crash after some updates
	crashCount := int32(0)
	createUI := func() (tea.Model, []tea.ProgramOption) {
		// Simulate UI that crashes every 3rd restart
		count := atomic.AddInt32(&crashCount, 1)
		shouldCrash := count%3 == 0

		return &mockUIModel{
				shouldPanic: shouldCrash,
				logger:      logger,
			}, []tea.ProgramOption{
				tea.WithoutSignalHandler(),
			}
	}

	// Start UI with recovery
	uiManager := ui.NewUIManager(logger, createUI)
	err := uiManager.Start()
	if err != nil {
		t.Fatalf("Failed to start UI: %v", err)
	}

	// Let the system run for a while
	testDuration := 2 * time.Second
	time.Sleep(testDuration)

	// Stop UI
	uiManager.Stop()

	// Stop sniper bot
	sniperBot.stop()
	time.Sleep(100 * time.Millisecond)

	// Verify statistics
	trades, prices := sniperBot.getStats()
	uiRestarts := uiManager.GetRestartCount()
	sent, dropped := ui.GlobalBus.GetStats()

	t.Logf("Test results after %v:", testDuration)
	t.Logf("  Trades executed: %d", trades)
	t.Logf("  Price updates: %d", prices)
	t.Logf("  UI restarts: %d", uiRestarts)
	t.Logf("  UI messages sent: %d, dropped: %d", sent, dropped)

	// Verify sniper continued operating
	expectedMinTrades := int32(testDuration / (50 * time.Millisecond))
	if trades < expectedMinTrades/2 {
		t.Errorf("Expected at least %d trades, got %d", expectedMinTrades/2, trades)
	}

	expectedMinPrices := int32(testDuration / (20 * time.Millisecond))
	if prices < expectedMinPrices/2 {
		t.Errorf("Expected at least %d price updates, got %d", expectedMinPrices/2, prices)
	}

	// Verify UI had some restarts
	if uiRestarts < 1 {
		t.Error("Expected UI to restart at least once")
	}

	// Verify non-blocking behavior
	if dropped == 0 && sent > 100 {
		t.Log("No messages dropped, UI handled all updates efficiently")
	}
}

// mockUIModel is a test UI model
type mockUIModel struct {
	shouldPanic bool
	updateCount int32
	logger      *zap.Logger
}

func (m *mockUIModel) Init() tea.Cmd {
	if m.shouldPanic {
		// Panic during init
		panic("UI init panic test")
	}
	return tea.Batch(
		tea.Every(10*time.Millisecond, func(t time.Time) tea.Msg {
			return time.Now()
		}),
		ui.ListenBus(),
	)
}

func (m *mockUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	count := atomic.AddInt32(&m.updateCount, 1)

	// Panic after some updates if configured
	if m.shouldPanic && count > 10 {
		panic("UI update panic test")
	}

	// Process messages
	switch msg := msg.(type) {
	case ui.PriceUpdateMsg:
		// Store in cache
		if state.GlobalCache != nil {
			state.GlobalCache.UpdatePosition("test_session", msg.Update)
		}
	case time.Time:
		// Check if we should quit
		if count > 50 {
			return m, tea.Quit
		}
	}

	return m, tea.Batch(
		tea.Every(10*time.Millisecond, func(t time.Time) tea.Msg {
			return time.Now()
		}),
		ui.ListenBus(),
	)
}

func (m *mockUIModel) View() string {
	return "Test UI Running"
}

// TestNonBlockingUpdates verifies UI updates don't block trading
func TestNonBlockingUpdates(t *testing.T) {
	logger := zap.NewNop()

	// Small channel to test blocking behavior
	msgChan := make(chan tea.Msg, 10)
	ui.InitBus(msgChan, logger)

	// Measure time to send many updates
	start := time.Now()
	for i := 0; i < 1000; i++ {
		ui.GlobalBus.Send(ui.LogMsg{
			Message: "Test message",
			Fields:  map[string]interface{}{"index": i},
		})
	}
	elapsed := time.Since(start)

	sent, dropped := ui.GlobalBus.GetStats()
	t.Logf("Sent 1000 messages in %v", elapsed)
	t.Logf("Actually sent: %d, dropped: %d", sent, dropped)

	// Should complete very quickly (non-blocking)
	if elapsed > 10*time.Millisecond {
		t.Errorf("Sending messages took too long: %v", elapsed)
	}

	// Should have dropped most messages
	if dropped < 900 {
		t.Errorf("Expected most messages to be dropped, only dropped %d", dropped)
	}
}
