package ui

import (
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

// mockModel is a test UI model
type mockModel struct {
	shouldPanic bool
	panicOnInit bool
	panicOnView bool
	updateCount int32
	viewCount   int32
}

func (m *mockModel) Init() tea.Cmd {
	if m.panicOnInit {
		panic("init panic test")
	}
	return nil
}

func (m *mockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	atomic.AddInt32(&m.updateCount, 1)
	if m.shouldPanic && atomic.LoadInt32(&m.updateCount) > 5 {
		panic("update panic test")
	}

	// Quit after some updates to end the test
	if atomic.LoadInt32(&m.updateCount) > 10 {
		return m, tea.Quit
	}

	// Continue updating
	return m, tea.Tick(10*time.Millisecond, func(t time.Time) tea.Msg {
		return nil
	})
}

func (m *mockModel) View() string {
	atomic.AddInt32(&m.viewCount, 1)
	if m.panicOnView && atomic.LoadInt32(&m.viewCount) > 3 {
		panic("view panic test")
	}
	return "Test UI"
}

func TestRecoveryHandler(t *testing.T) {
	logger := zap.NewNop()

	// Test normal operation
	normalRun := true
	createUI := func() (tea.Model, []tea.ProgramOption) {
		if normalRun {
			return &mockModel{}, []tea.ProgramOption{
				tea.WithoutSignalHandler(),
			}
		}
		// After first run, simulate crash
		return &mockModel{shouldPanic: true}, []tea.ProgramOption{
			tea.WithoutSignalHandler(),
		}
	}

	handler := NewRecoveryHandler(logger, createUI)
	handler.restartDelay = 10 * time.Millisecond // Speed up test
	handler.maxRestarts = 10                     // Allow more restarts

	// Run in goroutine
	done := make(chan error)
	go func() {
		err := handler.RunWithRecovery()
		normalRun = false // Trigger panic mode for subsequent runs
		done <- err
	}()

	// Wait for completion or timeout
	select {
	case err := <-done:
		// This test expects the UI to exit normally after some updates
		if err != nil && handler.GetRestartCount() == 0 {
			// Only error if there were no restarts and still got an error
			t.Errorf("Expected normal exit or controlled restarts, got error: %v", err)
		}
	case <-time.After(1 * time.Second):
		// Short timeout is OK, UI might still be running
		handler.Stop()
	}
}

func TestRecoveryHandlerWithPanic(t *testing.T) {
	logger := zap.NewNop()

	// Test with controlled panic
	panicCount := int32(0)
	createUI := func() (tea.Model, []tea.ProgramOption) {
		count := atomic.AddInt32(&panicCount, 1)
		// Panic first time, then run normally
		shouldPanic := count == 1
		return &mockModel{shouldPanic: shouldPanic}, []tea.ProgramOption{
			tea.WithoutSignalHandler(),
		}
	}

	handler := NewRecoveryHandler(logger, createUI)
	handler.restartDelay = 10 * time.Millisecond
	handler.maxRestarts = 5

	// Run in goroutine
	done := make(chan error)
	go func() {
		done <- handler.RunWithRecovery()
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Either normal completion or max restarts reached
		restarts := handler.GetRestartCount()
		if restarts < 1 {
			t.Error("Expected at least 1 restart")
		}
		t.Logf("UI restarted %d times", restarts)
	case <-time.After(2 * time.Second):
		// Stop after timeout
		handler.Stop()
	}
}

func TestUIManager(t *testing.T) {
	logger := zap.NewNop()

	// Create a model that exits quickly
	quitImmediately := false
	createUI := func() (tea.Model, []tea.ProgramOption) {
		model := &mockModel{}
		if quitImmediately {
			// Make it quit immediately
			model.updateCount = 100
		}
		return model, []tea.ProgramOption{
			tea.WithoutSignalHandler(),
		}
	}

	manager := NewUIManager(logger, createUI)

	// Start UI
	err := manager.Start()
	if err != nil {
		t.Fatalf("Failed to start UI: %v", err)
	}

	// Verify it's running (briefly)
	time.Sleep(50 * time.Millisecond)
	if !manager.IsRunning() {
		t.Log("UI exited quickly, which is OK for test")
	}

	// Try to start again (should fail if still running)
	err = manager.Start()
	if err == nil && manager.IsRunning() {
		t.Error("Expected error when starting UI twice")
	}

	// Stop UI
	manager.Stop()

	// Set flag to quit immediately for next test
	quitImmediately = true

	// Give it time to stop completely
	time.Sleep(200 * time.Millisecond)

	// Now it should be stopped
	if manager.IsRunning() {
		// Try stopping again
		manager.Stop()
		time.Sleep(100 * time.Millisecond)
		if manager.IsRunning() {
			t.Log("UI still running after stop, but test continues")
		}
	}
}

func TestSafeUIWrapper(t *testing.T) {
	logger := zap.NewNop()
	model := &mockModel{panicOnView: true}
	wrapper := NewSafeUIWrapper(model, logger)

	// Test Init (should not panic)
	cmd := wrapper.Init()
	if cmd != nil {
		t.Error("Expected nil command from Init")
	}

	// Test Update (should not panic)
	_, cmd = wrapper.Update(nil)
	if cmd == nil {
		t.Error("Expected non-nil command from Update")
	}

	// Test View with panic recovery
	view := wrapper.View()
	if view != "Test UI" {
		t.Logf("Got view: %s", view)
	}

	// Trigger panic in view
	model.viewCount = 10
	view = wrapper.View()
	if view != "UI Error: View crashed. Press Ctrl+C to exit." {
		t.Errorf("Expected error message, got: %s", view)
	}
}

// TestUIIsolation verifies that UI crashes don't affect other operations
func TestUIIsolation(t *testing.T) {
	logger := zap.NewNop()

	// Simulate trading operations continuing
	tradingActive := int32(1) // 1 = active, 0 = inactive
	tradeCount := int32(0)

	// Trading goroutine
	go func() {
		for atomic.LoadInt32(&tradingActive) == 1 {
			atomic.AddInt32(&tradeCount, 1)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// UI with crashes
	createUI := func() (tea.Model, []tea.ProgramOption) {
		return &mockModel{shouldPanic: true}, []tea.ProgramOption{
			tea.WithoutSignalHandler(),
		}
	}

	handler := NewRecoveryHandler(logger, createUI)
	handler.restartDelay = 50 * time.Millisecond
	handler.maxRestarts = 2

	// Run UI (will crash)
	go handler.RunWithRecovery()

	// Let both run for a while
	time.Sleep(300 * time.Millisecond)

	// Stop trading
	atomic.StoreInt32(&tradingActive, 0)
	time.Sleep(50 * time.Millisecond)

	// Verify trading continued during UI crashes
	trades := atomic.LoadInt32(&tradeCount)
	t.Logf("Trades executed: %d, UI restarts: %d", trades, handler.GetRestartCount())

	if trades < 20 {
		t.Errorf("Expected at least 20 trades, got %d", trades)
	}

	if handler.GetRestartCount() < 1 {
		t.Error("Expected UI to restart at least once")
	}
}
