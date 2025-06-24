package screen

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/bot"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/router"
	"github.com/rovshanmuradov/solana-bot/internal/ui/types"
	"go.uber.org/zap"
)

// Extensions to add real mode support to existing screens
// This approach uses composition to avoid modifying existing structs

// Task alias for convenience in UI code
type Task = task.Task

// RealModeMonitorScreen wraps MonitorScreen with real mode support
type RealModeMonitorScreen struct {
	*MonitorScreen
	serviceProvider ui.ServiceProvider
	realMode        bool
	positionMap     map[string]*types.MonitoredPosition // tokenMint -> position for quick updates
}

// NewRealModeMonitorScreen creates a monitor screen with real services
func NewRealModeMonitorScreen(serviceProvider ui.ServiceProvider) *RealModeMonitorScreen {
	base := NewMonitorScreen()
	screen := &RealModeMonitorScreen{
		MonitorScreen:   base,
		serviceProvider: serviceProvider,
		realMode:        true,
		positionMap:     make(map[string]*types.MonitoredPosition),
	}

	// Load real positions instead of mock data
	screen.loadRealPositions()

	// Subscribe to trading events
	screen.subscribeToEvents()

	return screen
}

// loadRealPositions sends command to load positions from trading service
func (s *RealModeMonitorScreen) loadRealPositions() {
	// Clear existing positions
	s.positions = make([]types.MonitoredPosition, 0)
	s.positionMap = make(map[string]*types.MonitoredPosition)

	// Send load positions command
	s.sendLoadPositionsCommand()
}

// subscribeToEvents subscribes to trading events from EventBus
func (s *RealModeMonitorScreen) subscribeToEvents() {
	eventBus := s.serviceProvider.GetEventBus()

	// Create UI event subscriber
	subscriber := &MonitorScreenEventSubscriber{screen: s}
	eventBus.Subscribe(subscriber)
}

// Update method with Command/Event pattern
func (s *RealModeMonitorScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			// Sell 100% of position using command
			if s.selectedPosition >= 0 && s.selectedPosition < len(s.positions) {
				position := s.positions[s.selectedPosition]
				return s, s.sendSellCommand(position, 100.0)
			}
		case "1", "2", "3", "4", "5":
			// Quick sell percentages using commands
			if s.selectedPosition >= 0 && s.selectedPosition < len(s.positions) {
				position := s.positions[s.selectedPosition]
				percentage := s.getQuickSellPercentage(msg.String())
				return s, s.sendSellCommand(position, percentage)
			}
		case "r":
			// Refresh positions using command
			return s, s.sendLoadPositionsCommand()
		}

	// Handle trading events
	case bot.PositionUpdatedEvent:
		s.handlePositionUpdatedEvent(msg)
	case bot.SellCompletedEvent:
		s.handleSellCompletedEvent(msg)
	case bot.MonitoringSessionStoppedEvent:
		s.handleSessionStoppedEvent(msg)
	case bot.PositionsLoadedEvent:
		s.handlePositionsLoadedEvent(msg)
	}

	// Delegate to base screen for other messages
	baseScreen, baseCmd := s.MonitorScreen.Update(msg)
	if baseScreen != s.MonitorScreen {
		// If base screen changed, wrap it again
		return s, baseCmd
	}
	return s, baseCmd
}

// sendSellCommand sends a sell position command
func (s *RealModeMonitorScreen) sendSellCommand(position types.MonitoredPosition, percentage float64) tea.Cmd {
	return func() tea.Msg {
		commandBus := s.serviceProvider.GetCommandBus()
		logger := s.serviceProvider.GetLogger()

		cmd := bot.SellPositionCommand{
			TokenMint:  position.TokenMint,
			Percentage: percentage,
			UserID:     "ui_user",
			Timestamp:  time.Now(),
		}

		logger.Info("🔄 Sending sell command from UI",
			zap.String("token", position.TokenMint),
			zap.Float64("percentage", percentage))

		// Send command to trading service
		ctx := context.Background()
		if err := commandBus.Send(ctx, cmd); err != nil {
			logger.Error("❌ Failed to send sell command", zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Sell Command Failed"}
		}

		return ui.SuccessMsg{Message: fmt.Sprintf("Sell command sent: %.1f%% of %s", percentage, position.TokenSymbol)}
	}
}

// sendLoadPositionsCommand sends a load positions command
func (s *RealModeMonitorScreen) sendLoadPositionsCommand() tea.Cmd {
	return func() tea.Msg {
		commandBus := s.serviceProvider.GetCommandBus()
		logger := s.serviceProvider.GetLogger()

		cmd := bot.LoadPositionsCommand{
			UserID:    "ui_user",
			Timestamp: time.Now(),
		}

		logger.Info("📊 Sending load positions command from UI")

		// Send command to trading service
		ctx := context.Background()
		if err := commandBus.Send(ctx, cmd); err != nil {
			logger.Error("❌ Failed to send load positions command", zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Load Positions Command Failed"}
		}

		return nil // Event handler will update UI
	}
}

// getQuickSellPercentage returns percentage for quick sell hotkeys
func (s *RealModeMonitorScreen) getQuickSellPercentage(key string) float64 {
	percentages := map[string]float64{
		"1": 25.0,
		"2": 50.0,
		"3": 75.0,
		"4": 90.0,
		"5": 100.0,
	}
	return percentages[key]
}

// Event handlers

// handlePositionUpdatedEvent handles position updated events
func (s *RealModeMonitorScreen) handlePositionUpdatedEvent(event bot.PositionUpdatedEvent) {
	if position, exists := s.positionMap[event.TokenMint]; exists {
		// Update position data from event
		position.CurrentPrice = event.CurrentPrice
		position.EntryPrice = event.EntryPrice
		position.PnLPercent = event.PnLPercent
		position.PnLSol = event.PnLSol
		position.Amount = event.Amount
		position.LastUpdate = event.Timestamp
		position.TokenSymbol = event.TokenSymbol

		// Add to price history
		if len(position.PriceHistory) >= 20 {
			position.PriceHistory = position.PriceHistory[1:]
		}
		position.PriceHistory = append(position.PriceHistory, event.CurrentPrice)

		// Update table display
		s.updateTableDisplay()
	}
}

// handleSellCompletedEvent handles sell completed events
func (s *RealModeMonitorScreen) handleSellCompletedEvent(event bot.SellCompletedEvent) {
	logger := s.serviceProvider.GetLogger()

	if event.Success {
		logger.Info("✅ Sell completed",
			zap.String("token", event.TokenMint),
			zap.Float64("amount_sold", event.AmountSold),
			zap.Float64("sol_received", event.SolReceived))

		// Show success message in UI
		ui.PublishSuccess(fmt.Sprintf("Sold %.6f tokens for %.6f SOL",
			event.AmountSold, event.SolReceived), "Sell Completed")
	} else {
		logger.Error("❌ Sell failed", zap.String("error", event.Error))
		ui.PublishError(fmt.Errorf(event.Error), "Sell Failed")
	}
}

// handleSessionStoppedEvent handles monitoring session stopped events
func (s *RealModeMonitorScreen) handleSessionStoppedEvent(event bot.MonitoringSessionStoppedEvent) {
	// Remove position from UI if session stopped
	if position, exists := s.positionMap[event.TokenMint]; exists {
		position.Active = false

		// Find and remove from positions slice
		for i, pos := range s.positions {
			if pos.TokenMint == event.TokenMint {
				s.positions = append(s.positions[:i], s.positions[i+1:]...)
				break
			}
		}

		delete(s.positionMap, event.TokenMint)
		s.updateTableDisplay()
	}
}

// handlePositionsLoadedEvent handles positions loaded events
func (s *RealModeMonitorScreen) handlePositionsLoadedEvent(event bot.PositionsLoadedEvent) {
	// Convert UIPosition to MonitoredPosition
	s.positions = make([]types.MonitoredPosition, len(event.Positions))
	s.positionMap = make(map[string]*types.MonitoredPosition)

	for i, uiPos := range event.Positions {
		position := types.MonitoredPosition{
			ID:           uiPos.ID,
			TaskName:     uiPos.TaskName,
			TokenMint:    uiPos.TokenMint,
			TokenSymbol:  uiPos.TokenSymbol,
			EntryPrice:   uiPos.EntryPrice,
			CurrentPrice: uiPos.CurrentPrice,
			Amount:       uiPos.Amount,
			PnLPercent:   uiPos.PnLPercent,
			PnLSol:       uiPos.PnLSol,
			Volume24h:    uiPos.Volume24h,
			LastUpdate:   uiPos.LastUpdate,
			PriceHistory: uiPos.PriceHistory,
			Active:       uiPos.Active,
		}

		s.positions[i] = position
		s.positionMap[position.TokenMint] = &s.positions[i]
	}

	// Update table display
	s.updateTableDisplay()
}

// RealModeTaskListScreen wraps TaskListScreen with real mode support
type RealModeTaskListScreen struct {
	*TaskListScreen
	serviceProvider ui.ServiceProvider
	realMode        bool
}

// NewRealModeTaskListScreen creates a task list screen with real services
func NewRealModeTaskListScreen(serviceProvider ui.ServiceProvider) *RealModeTaskListScreen {
	base := NewTaskListScreen()
	screen := &RealModeTaskListScreen{
		TaskListScreen:  base,
		serviceProvider: serviceProvider,
		realMode:        true,
	}

	// Load real tasks instead of mock data
	screen.loadRealTasks()

	// Subscribe to trading events
	screen.subscribeToEvents()

	return screen
}

// loadRealTasks sends command to load tasks from trading service
func (s *RealModeTaskListScreen) loadRealTasks() {
	// Clear existing tasks
	s.tasks = make([]task.Task, 0)

	// Send load tasks command
	s.sendLoadTasksCommand()
}

// subscribeToEvents subscribes to trading events from EventBus
func (s *RealModeTaskListScreen) subscribeToEvents() {
	eventBus := s.serviceProvider.GetEventBus()

	// Create UI event subscriber
	subscriber := &TaskListEventSubscriber{screen: s}
	eventBus.Subscribe(subscriber)
}

// Update method with Command/Event pattern
func (s *RealModeTaskListScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Execute selected task using command
			selectedRow := s.table.GetSelectedRow()
			if selectedRow >= 0 && selectedRow < len(s.tasks) {
				task := s.tasks[selectedRow]
				return s, s.sendExecuteTaskCommand(task)
			}
		case "r":
			// Refresh tasks using command
			return s, s.sendLoadTasksCommand()
		}

	// Handle trading events
	case bot.TaskExecutedEvent:
		s.handleTaskExecutedEvent(msg)
	case bot.PositionCreatedEvent:
		s.handlePositionCreatedEvent(msg)
	case bot.TasksLoadedEvent:
		s.handleTasksLoadedEvent(msg)
	}

	// Delegate to base screen for other messages
	baseScreen, baseCmd := s.TaskListScreen.Update(msg)
	if baseScreen != s.TaskListScreen {
		// If base screen changed, wrap it again
		return s, baseCmd
	}
	return s, baseCmd
}

// sendExecuteTaskCommand sends an execute task command
func (s *RealModeTaskListScreen) sendExecuteTaskCommand(uiTask Task) tea.Cmd {
	return func() tea.Msg {
		commandBus := s.serviceProvider.GetCommandBus()
		logger := s.serviceProvider.GetLogger()

		cmd := bot.ExecuteTaskCommand{
			TaskID:    uiTask.ID,
			UserID:    "ui_user",
			Timestamp: time.Now(),
		}

		logger.Info("🚀 Sending execute task command from UI",
			zap.Int("task_id", uiTask.ID),
			zap.String("task_name", uiTask.TaskName),
			zap.String("token", uiTask.TokenMint))

		// Send command to trading service
		ctx := context.Background()
		if err := commandBus.Send(ctx, cmd); err != nil {
			logger.Error("❌ Failed to send execute task command", zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Execute Task Command Failed"}
		}

		return ui.SuccessMsg{Message: fmt.Sprintf("Task %s execution started", uiTask.TaskName)}
	}
}

// sendLoadTasksCommand sends a load tasks command
func (s *RealModeTaskListScreen) sendLoadTasksCommand() tea.Cmd {
	return func() tea.Msg {
		commandBus := s.serviceProvider.GetCommandBus()
		logger := s.serviceProvider.GetLogger()

		cmd := bot.LoadTasksCommand{
			UserID:    "ui_user",
			Timestamp: time.Now(),
		}

		logger.Info("📋 Sending load tasks command from UI")

		// Send command to trading service
		ctx := context.Background()
		if err := commandBus.Send(ctx, cmd); err != nil {
			logger.Error("❌ Failed to send load tasks command", zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Load Tasks Command Failed"}
		}

		return nil // Event handler will update UI
	}
}

// Event handlers

// handleTaskExecutedEvent handles task executed events
func (s *RealModeTaskListScreen) handleTaskExecutedEvent(event bot.TaskExecutedEvent) {
	logger := s.serviceProvider.GetLogger()

	if event.Success {
		logger.Info("✅ Task executed successfully",
			zap.Int("task_id", event.TaskID),
			zap.String("task_name", event.TaskName),
			zap.String("tx_signature", event.TxSignature))

		ui.PublishSuccess(fmt.Sprintf("Task %s completed successfully", event.TaskName), "Task Completed")
	} else {
		logger.Error("❌ Task execution failed",
			zap.Int("task_id", event.TaskID),
			zap.String("error", event.Error))

		ui.PublishError(fmt.Errorf(event.Error), fmt.Sprintf("Task %s Failed", event.TaskName))
	}
}

// handlePositionCreatedEvent handles position created events
func (s *RealModeTaskListScreen) handlePositionCreatedEvent(event bot.PositionCreatedEvent) {
	logger := s.serviceProvider.GetLogger()

	logger.Info("📊 Position created",
		zap.Int("task_id", event.TaskID),
		zap.String("token", event.TokenMint),
		zap.Float64("entry_price", event.EntryPrice),
		zap.Uint64("token_balance", event.TokenBalance))

	ui.PublishSuccess(fmt.Sprintf("Position created for %s", event.TokenSymbol), "Position Created")
}

// handleTasksLoadedEvent handles tasks loaded events
func (s *RealModeTaskListScreen) handleTasksLoadedEvent(event bot.TasksLoadedEvent) {
	// Convert UITask to task.Task
	s.tasks = make([]task.Task, len(event.Tasks))

	for i, uiTask := range event.Tasks {
		s.tasks[i] = task.Task{
			ID:             uiTask.ID,
			TaskName:       uiTask.TaskName,
			TokenMint:      uiTask.TokenMint,
			Operation:      task.OperationType(uiTask.ActionType),
			AmountSol:      uiTask.Amount,
			AutosellAmount: uiTask.AutosellAmount,
			WalletName:     uiTask.WalletKey,
			// Set other fields to defaults as needed
			Module:          "snipe",
			SlippagePercent: 5.0,
			PriorityFeeSol:  "0.000002",
			ComputeUnits:    200000,
		}
	}

	// Update table display
	s.updateTableDisplay()
}

// Event subscribers

// MonitorScreenEventSubscriber handles events for monitor screen
type MonitorScreenEventSubscriber struct {
	screen *RealModeMonitorScreen
}

func (s *MonitorScreenEventSubscriber) OnEvent(event bot.TradingEvent) {
	switch e := event.(type) {
	case bot.PositionUpdatedEvent:
		s.screen.handlePositionUpdatedEvent(e)
	case bot.SellCompletedEvent:
		s.screen.handleSellCompletedEvent(e)
	case bot.MonitoringSessionStoppedEvent:
		s.screen.handleSessionStoppedEvent(e)
	case bot.PositionsLoadedEvent:
		s.screen.handlePositionsLoadedEvent(e)
	}
}

func (s *MonitorScreenEventSubscriber) GetSubscribedEventTypes() []string {
	return []string{
		"position_updated",
		"sell_completed",
		"monitoring_session_stopped",
		"positions_loaded",
	}
}

// TaskListEventSubscriber handles events for task list screen
type TaskListEventSubscriber struct {
	screen *RealModeTaskListScreen
}

func (s *TaskListEventSubscriber) OnEvent(event bot.TradingEvent) {
	switch e := event.(type) {
	case bot.TaskExecutedEvent:
		s.screen.handleTaskExecutedEvent(e)
	case bot.PositionCreatedEvent:
		s.screen.handlePositionCreatedEvent(e)
	case bot.TasksLoadedEvent:
		s.screen.handleTasksLoadedEvent(e)
	}
}

func (s *TaskListEventSubscriber) GetSubscribedEventTypes() []string {
	return []string{
		"task_executed",
		"position_created",
		"tasks_loaded",
	}
}
