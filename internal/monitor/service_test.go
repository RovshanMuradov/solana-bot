// internal/monitor/service_test.go
package monitor

import (
	"context"
	"testing"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap/zaptest"
)

// MockEventBus for testing
type MockEventBus struct {
	events []interface{}
}

func (m *MockEventBus) Publish(event interface{}) {
	m.events = append(m.events, event)
}

func (m *MockEventBus) GetEvents() []interface{} {
	return m.events
}

func TestNewMonitorService(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config)

	if service == nil {
		t.Fatal("NewMonitorService returned nil")
	}

	// Test initial state
	sessions := service.GetAllSessions()
	if len(sessions) != 0 {
		t.Fatal("Expected empty sessions map initially")
	}
}

func TestMonitorService_ValidateCreateRequest(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config).(*MonitorServiceImpl)

	// Test nil request
	err := service.validateCreateRequest(nil)
	if err == nil {
		t.Fatal("Expected error for nil request")
	}

	// Test request with nil task
	req := &CreateSessionRequest{}
	err = service.validateCreateRequest(req)
	if err == nil {
		t.Fatal("Expected error for nil task")
	}

	// Test request with empty token mint
	testWallet, _ := task.NewWallet("5XEzKqUuGzyKN9hBr5cQjUH6gWVxHfaQKPZ2pD2ycCBdNRGC7u7wAULhCE1mBGsHPe7JVXG8V9bZvUoSSrV9yVQA")
	req = &CreateSessionRequest{
		Task: &task.Task{
			TokenMint: "",
		},
		Wallet:     testWallet,
		DEXName:    "snipe",
		EntryPrice: 1.0,
	}
	err = service.validateCreateRequest(req)
	if err == nil {
		t.Fatal("Expected error for empty token mint")
	}

	// Test valid request
	req = &CreateSessionRequest{
		Task: &task.Task{
			TokenMint: "test_token_mint",
		},
		Wallet:     testWallet,
		DEXName:    "snipe",
		EntryPrice: 1.0,
	}
	err = service.validateCreateRequest(req)
	if err != nil {
		t.Fatalf("Expected no error for valid request, got: %v", err)
	}
}

func TestMonitorService_GetSession(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config)

	// Test get non-existent session
	_, exists := service.GetSession("non_existent_token")
	if exists {
		t.Fatal("Session should not exist")
	}

	// Test get all sessions
	sessions := service.GetAllSessions()
	if len(sessions) != 0 {
		t.Fatal("Expected 0 sessions initially")
	}
}

func TestMonitorService_StopMonitoringSession(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config)

	// Test stop non-existent session
	err := service.StopMonitoringSession("non_existent_token")
	if err == nil {
		t.Fatal("Expected error for stopping non-existent session")
	}
}

func TestMonitorService_RemoveSession(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config)

	// Test remove non-existent session (should not error)
	service.RemoveSession("non_existent_token")

	// Verify sessions are still empty
	sessions := service.GetAllSessions()
	if len(sessions) != 0 {
		t.Fatal("Expected 0 sessions after removing non-existent session")
	}
}

func TestMonitorService_Shutdown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config)

	// Test shutdown with no sessions
	ctx := context.Background()
	err := service.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Expected no error on shutdown, got: %v", err)
	}

	// Verify sessions are empty after shutdown
	sessions := service.GetAllSessions()
	if len(sessions) != 0 {
		t.Fatal("Expected 0 sessions after shutdown")
	}
}

func TestMonitorService_EventPublishing(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config).(*MonitorServiceImpl)

	// Test publishSessionStartedEvent
	testWallet, _ := task.NewWallet("5XEzKqUuGzyKN9hBr5cQjUH6gWVxHfaQKPZ2pD2ycCBdNRGC7u7wAULhCE1mBGsHPe7JVXG8V9bZvUoSSrV9yVQA")
	req := &CreateSessionRequest{
		Task: &task.Task{
			TokenMint: "test_token_mint",
		},
		EntryPrice:   1.0,
		TokenBalance: 1000000,
		Wallet:       testWallet,
		DEXName:      "snipe",
		UserID:       "test_user",
	}

	service.publishSessionStartedEvent(req)

	events := eventBus.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	startedEvent, ok := events[0].(MonitoringSessionStartedEvent)
	if !ok {
		t.Fatal("Expected MonitoringSessionStartedEvent")
	}

	if startedEvent.TokenMint != "test_token_mint" {
		t.Errorf("Expected token mint 'test_token_mint', got %s", startedEvent.TokenMint)
	}

	if startedEvent.InitialPrice != 1.0 {
		t.Errorf("Expected initial price 1.0, got %f", startedEvent.InitialPrice)
	}

	if startedEvent.UserID != "test_user" {
		t.Errorf("Expected user ID 'test_user', got %s", startedEvent.UserID)
	}
}

func TestMonitorService_PositionUpdatedEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config).(*MonitorServiceImpl)

	// Test publishPositionUpdatedEvent
	update := PriceUpdate{
		Current: 1.5,
		Initial: 1.0,
		Percent: 50.0,
		Tokens:  1000.0,
	}

	service.publishPositionUpdatedEvent("test_token_mint", update, "test_user")

	events := eventBus.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	updatedEvent, ok := events[0].(PositionUpdatedEvent)
	if !ok {
		t.Fatal("Expected PositionUpdatedEvent")
	}

	if updatedEvent.TokenMint != "test_token_mint" {
		t.Errorf("Expected token mint 'test_token_mint', got %s", updatedEvent.TokenMint)
	}

	if updatedEvent.CurrentPrice != 1.5 {
		t.Errorf("Expected current price 1.5, got %f", updatedEvent.CurrentPrice)
	}

	if updatedEvent.EntryPrice != 1.0 {
		t.Errorf("Expected entry price 1.0, got %f", updatedEvent.EntryPrice)
	}

	if updatedEvent.PnLPercent != 50.0 {
		t.Errorf("Expected PnL percent 50.0, got %f", updatedEvent.PnLPercent)
	}

	expectedPnLSol := (1.5 - 1.0) * 1000.0 // (current - initial) * tokens
	if updatedEvent.PnLSol != expectedPnLSol {
		t.Errorf("Expected PnL SOL %f, got %f", expectedPnLSol, updatedEvent.PnLSol)
	}
}

func TestMonitorService_SessionStoppedEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config).(*MonitorServiceImpl)

	// Test publishSessionStoppedEvent
	service.publishSessionStoppedEvent("test_token_mint", "manual_stop")

	events := eventBus.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	stoppedEvent, ok := events[0].(MonitoringSessionStoppedEvent)
	if !ok {
		t.Fatal("Expected MonitoringSessionStoppedEvent")
	}

	if stoppedEvent.TokenMint != "test_token_mint" {
		t.Errorf("Expected token mint 'test_token_mint', got %s", stoppedEvent.TokenMint)
	}

	if stoppedEvent.Reason != "manual_stop" {
		t.Errorf("Expected reason 'manual_stop', got %s", stoppedEvent.Reason)
	}

	if stoppedEvent.UserID != "system" {
		t.Errorf("Expected user ID 'system', got %s", stoppedEvent.UserID)
	}
}

func TestMonitorService_GetTokenSymbol(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config).(*MonitorServiceImpl)

	// Test token symbol generation
	tokenMint := "abcdef1234567890"
	symbol := service.getTokenSymbol(tokenMint)
	expected := "abcd...7890"
	if symbol != expected {
		t.Errorf("Expected symbol '%s', got '%s'", expected, symbol)
	}

	// Test short token mint
	shortMint := "abc"
	symbol = service.getTokenSymbol(shortMint)
	if symbol != "TOKEN" {
		t.Errorf("Expected symbol 'TOKEN' for short mint, got '%s'", symbol)
	}
}

func TestMonitorService_CreateSessionRequest_DefaultInterval(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockchainClient := blockchain.NewClient("https://api.devnet.solana.com", logger)
	eventBus := &MockEventBus{}

	config := &MonitorServiceConfig{
		Logger:           logger,
		BlockchainClient: blockchainClient,
		EventBus:         eventBus,
	}

	service := NewMonitorService(config).(*MonitorServiceImpl)

	// Mock the validation to pass by providing valid request data
	testWallet, _ := task.NewWallet("5XEzKqUuGzyKN9hBr5cQjUH6gWVxHfaQKPZ2pD2ycCBdNRGC7u7wAULhCE1mBGsHPe7JVXG8V9bZvUoSSrV9yVQA")
	req := &CreateSessionRequest{
		Task: &task.Task{
			TokenMint: "test_token_mint",
			Module:    "snipe",
		},
		EntryPrice:   1.0,
		TokenBalance: 1000000,
		Wallet:       testWallet,
		DEXName:      "snipe",
		Interval:     0, // Default interval should be applied
		UserID:       "test_user",
	}

	// Test validation passes for valid request
	err := service.validateCreateRequest(req)
	if err != nil {
		t.Fatalf("Validation should pass for valid request, got: %v", err)
	}

	// Note: Full CreateMonitoringSession test would require mocking DEX creation
	// which is complex and beyond the scope of this unit test
}
