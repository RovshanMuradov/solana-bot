package ui

import (
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

func TestUpdateSenderNonBlocking(t *testing.T) {
	logger := zap.NewNop()
	msgChan := make(chan tea.Msg, 10) // Small buffer to test blocking
	sender := NewUpdateSender(msgChan, logger)
	defer sender.Close()

	// Fill the channel
	for i := 0; i < 10; i++ {
		sender.SendUpdate(LogMsg{Message: "test"})
	}

	// These should be dropped without blocking
	start := time.Now()
	for i := 0; i < 100; i++ {
		sender.SendUpdate(LogMsg{Message: "dropped"})
	}
	elapsed := time.Since(start)

	// Should complete quickly (non-blocking)
	if elapsed > 100*time.Millisecond {
		t.Errorf("SendUpdate blocked for %v, expected non-blocking", elapsed)
	}

	sent, dropped := sender.GetStats()
	t.Logf("Sent: %d, Dropped: %d", sent, dropped)

	if dropped == 0 {
		t.Error("Expected some messages to be dropped")
	}
}

func TestUpdateSenderConcurrent(t *testing.T) {
	logger := zap.NewNop()
	msgChan := make(chan tea.Msg, 100)
	sender := NewUpdateSender(msgChan, logger)
	defer sender.Close()

	var wg sync.WaitGroup
	numGoroutines := 10
	messagesPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				sender.SendUpdate(LogMsg{
					Message: "test",
					Fields:  map[string]interface{}{"id": id, "seq": j},
				})
			}
		}(i)
	}

	wg.Wait()

	sent, dropped := sender.GetStats()
	total := sent + dropped
	expected := uint64(numGoroutines * messagesPerGoroutine)

	if total != expected {
		t.Errorf("Expected %d total messages, got %d (sent: %d, dropped: %d)",
			expected, total, sent, dropped)
	}
}

func TestGlobalBus(t *testing.T) {
	logger := zap.NewNop()
	msgChan := make(chan tea.Msg, 50)
	InitBus(msgChan, logger)
	defer GlobalBus.Close()

	// Send messages
	for i := 0; i < 100; i++ {
		GlobalBus.Send(LogMsg{Message: "test"})
	}

	sent, dropped := GlobalBus.GetStats()
	t.Logf("Global bus - Sent: %d, Dropped: %d", sent, dropped)

	if sent == 0 {
		t.Error("Expected some messages to be sent")
	}
}
