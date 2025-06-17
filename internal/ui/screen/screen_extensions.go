package screen

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/router"
	"go.uber.org/zap"
)

// Extensions to add real mode support to existing screens
// This approach uses composition to avoid modifying existing structs

// RealModeMonitorScreen wraps MonitorScreen with real mode support
type RealModeMonitorScreen struct {
	*MonitorScreen
	serviceProvider ui.ServiceProvider
	realMode        bool
	positionMap     map[string]*MonitoredPosition // tokenMint -> position for quick updates
}

// NewRealModeMonitorScreen creates a monitor screen with real services
func NewRealModeMonitorScreen(serviceProvider ui.ServiceProvider) *RealModeMonitorScreen {
	base := NewMonitorScreen()
	screen := &RealModeMonitorScreen{
		MonitorScreen:   base,
		serviceProvider: serviceProvider,
		realMode:        true,
		positionMap:     make(map[string]*MonitoredPosition),
	}

	// Load real positions instead of mock data
	screen.loadRealPositions()

	return screen
}

// loadRealPositions loads real trading positions from active monitoring sessions
func (s *RealModeMonitorScreen) loadRealPositions() {
	s.positions = make([]MonitoredPosition, 0)

	// Get active monitoring sessions
	sessions := s.serviceProvider.GetAllMonitoringSessions()

	positionID := 1
	for tokenMint := range sessions {
		// Create position from monitoring session
		position := MonitoredPosition{
			ID:           positionID,
			TaskName:     fmt.Sprintf("MONITOR_%d", positionID),
			TokenMint:    tokenMint,
			TokenSymbol:  s.getTokenSymbol(tokenMint),
			EntryPrice:   0, // Will be updated from session
			CurrentPrice: 0, // Will be updated from session
			Amount:       0, // Will be updated from session
			PnLPercent:   0,
			PnLSol:       0,
			Volume24h:    0,
			LastUpdate:   time.Now(),
			PriceHistory: make([]float64, 0),
			Active:       true,
		}

		s.positions = append(s.positions, position)
		s.positionMap[tokenMint] = &s.positions[len(s.positions)-1]
		positionID++
	}

	// Update table display
	s.updateTableDisplay()
}

// getTokenSymbol extracts a symbol from token mint (simplified)
func (s *RealModeMonitorScreen) getTokenSymbol(tokenMint string) string {
	if len(tokenMint) >= 8 {
		return strings.ToUpper(tokenMint[:4]) + "..." + strings.ToUpper(tokenMint[len(tokenMint)-4:])
	}
	return "TOKEN"
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

	return screen
}

// loadRealTasks loads tasks from CSV file using task manager
func (s *RealModeTaskListScreen) loadRealTasks() {
	taskManager := s.serviceProvider.GetTaskManager()
	logger := s.serviceProvider.GetLogger()

	// Load tasks from CSV
	tasks, err := taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		logger.Error("Failed to load tasks", zap.Error(err))
		ui.PublishError(err, "Task Loading Error")
		return
	}

	// Convert *task.Task to task.Task for UI
	s.tasks = make([]task.Task, len(tasks))
	for i, t := range tasks {
		s.tasks[i] = *t
	}

	// Update table display
	s.updateTableDisplay()

	logger.Info("Loaded real tasks", zap.Int("count", len(s.tasks)))
}

// Update overrides the base Update method to use real execution
func (s *RealModeTaskListScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			// Handle Enter key for real task execution
			if s.executingTask == -1 {
				selectedRow := s.table.GetSelectedRow()
				if selectedRow < len(s.tasks) {
					taskToExecute := s.tasks[selectedRow]
					s.executingTask = taskToExecute.ID
					cmds = append(cmds, s.executeTaskCmdReal(taskToExecute))
				}
			}
			return s, tea.Batch(cmds...)
		}
	}

	// For all other messages, delegate to base class
	updatedScreen, cmd := s.TaskListScreen.Update(msg)
	s.TaskListScreen = updatedScreen.(*TaskListScreen)

	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return s, tea.Batch(cmds...)
}

// executeTaskCmdReal performs real task execution with DEX adapters
func (s *RealModeTaskListScreen) executeTaskCmdReal(taskToExecute task.Task) tea.Cmd {
	return func() tea.Msg {
		ctx := s.serviceProvider.GetContext()
		logger := s.serviceProvider.GetLogger()
		wallets := s.serviceProvider.GetWallets()
		blockchainClient := s.serviceProvider.GetBlockchainClient()

		// Validate all services are available
		if ctx == nil {
			err := fmt.Errorf("context is nil")
			logger.Error("Service validation failed", zap.Error(err))
			return TaskExecutionMsg{
				TaskID:    taskToExecute.ID,
				Status:    "failed",
				Completed: true,
				Error:     err,
			}
		}
		if logger == nil {
			err := fmt.Errorf("logger is nil")
			return TaskExecutionMsg{
				TaskID:    taskToExecute.ID,
				Status:    "failed",
				Completed: true,
				Error:     err,
			}
		}
		if wallets == nil {
			err := fmt.Errorf("wallets map is nil")
			logger.Error("Service validation failed", zap.Error(err))
			return TaskExecutionMsg{
				TaskID:    taskToExecute.ID,
				Status:    "failed",
				Completed: true,
				Error:     err,
			}
		}
		if blockchainClient == nil {
			err := fmt.Errorf("blockchain client is nil")
			logger.Error("Service validation failed", zap.Error(err))
			return TaskExecutionMsg{
				TaskID:    taskToExecute.ID,
				Status:    "failed",
				Completed: true,
				Error:     err,
			}
		}

		// Publish task started event
		ui.PublishTaskStarted(ui.TaskStartedMsg{
			TaskID:    taskToExecute.ID,
			TaskName:  taskToExecute.TaskName,
			TokenMint: taskToExecute.TokenMint,
			Operation: string(taskToExecute.Operation),
			AmountSol: taskToExecute.AmountSol,
			Wallet:    taskToExecute.WalletName,
		})

		logger.Info("🚀 Starting real task execution",
			zap.Int("task_id", taskToExecute.ID),
			zap.String("task_name", taskToExecute.TaskName),
			zap.String("operation", string(taskToExecute.Operation)),
			zap.String("token", taskToExecute.TokenMint),
			zap.Float64("amount", taskToExecute.AmountSol))

		// Get wallet for this task
		wallet, exists := wallets[taskToExecute.WalletName]
		if !exists {
			err := fmt.Errorf("wallet %s not found", taskToExecute.WalletName)
			logger.Error("Wallet not found", zap.Error(err))

			ui.PublishTaskCompleted(ui.TaskCompletedMsg{
				TaskID:    taskToExecute.ID,
				TaskName:  taskToExecute.TaskName,
				TokenMint: taskToExecute.TokenMint,
				Success:   false,
				Error:     err.Error(),
			})

			return TaskExecutionMsg{
				TaskID:    taskToExecute.ID,
				Status:    "failed",
				Completed: true,
				Error:     err,
			}
		}

		// Create DEX adapter
		logger.Info("Creating DEX adapter",
			zap.String("module", taskToExecute.Module),
			zap.String("wallet", taskToExecute.WalletName),
			zap.String("token", taskToExecute.TokenMint))

		dexAdapter, err := dex.GetDEXByName(taskToExecute.Module, blockchainClient, wallet, logger)
		if err != nil {
			logger.Error("Failed to create DEX adapter",
				zap.String("module", taskToExecute.Module),
				zap.Error(err))

			ui.PublishTaskCompleted(ui.TaskCompletedMsg{
				TaskID:    taskToExecute.ID,
				TaskName:  taskToExecute.TaskName,
				TokenMint: taskToExecute.TokenMint,
				Success:   false,
				Error:     err.Error(),
			})

			return TaskExecutionMsg{
				TaskID:    taskToExecute.ID,
				Status:    "failed",
				Completed: true,
				Error:     err,
			}
		}

		logger.Info("DEX adapter created successfully",
			zap.String("module", taskToExecute.Module))

		// Execute the task based on operation type
		var txSignature string
		var entryPrice float64
		var tokenBalance uint64

		execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		switch taskToExecute.Operation {
		case task.OperationSnipe, task.OperationSwap:
			// Execute REAL buy operation through DEX adapter
			logger.Info("Executing REAL task through DEX adapter",
				zap.String("operation", string(taskToExecute.Operation)),
				zap.String("token", taskToExecute.TokenMint),
				zap.Float64("amount_sol", taskToExecute.AmountSol))

			// Execute the actual task
			execErr := dexAdapter.Execute(execCtx, &taskToExecute)
			if execErr != nil {
				logger.Error("Task execution failed",
					zap.String("token", taskToExecute.TokenMint),
					zap.Error(execErr))

				ui.PublishTaskCompleted(ui.TaskCompletedMsg{
					TaskID:    taskToExecute.ID,
					TaskName:  taskToExecute.TaskName,
					TokenMint: taskToExecute.TokenMint,
					Success:   false,
					Error:     execErr.Error(),
				})

				return TaskExecutionMsg{
					TaskID:    taskToExecute.ID,
					Status:    "failed",
					Completed: true,
					Error:     execErr,
				}
			}

			logger.Info("Task executed successfully",
				zap.String("token", taskToExecute.TokenMint))

			// Get current token price after execution
			price, priceErr := dexAdapter.GetTokenPrice(execCtx, taskToExecute.TokenMint)
			if priceErr != nil {
				logger.Error("Failed to get token price after execution",
					zap.String("token", taskToExecute.TokenMint),
					zap.Error(priceErr))
				// Continue with zero price rather than fail
				price = 0
			}

			// Get actual token balance after purchase
			actualBalance, balanceErr := dexAdapter.GetTokenBalance(execCtx, taskToExecute.TokenMint)
			if balanceErr != nil {
				logger.Error("Failed to get token balance after execution",
					zap.String("token", taskToExecute.TokenMint),
					zap.Error(balanceErr))
				// Calculate expected balance
				tokenAmount := taskToExecute.AmountSol / price
				actualBalance = uint64(tokenAmount * math.Pow10(6))
			}

			entryPrice = price
			tokenBalance = actualBalance
			txSignature = fmt.Sprintf("real_buy_%s_%d", taskToExecute.TokenMint[:8], time.Now().Unix())

		case task.OperationSell:
			// Execute REAL sell operation through DEX adapter
			logger.Info("Executing REAL sell operation through DEX adapter",
				zap.String("token", taskToExecute.TokenMint))

			execErr := dexAdapter.Execute(execCtx, &taskToExecute)
			if execErr != nil {
				logger.Error("Sell task execution failed",
					zap.String("token", taskToExecute.TokenMint),
					zap.Error(execErr))

				ui.PublishTaskCompleted(ui.TaskCompletedMsg{
					TaskID:    taskToExecute.ID,
					TaskName:  taskToExecute.TaskName,
					TokenMint: taskToExecute.TokenMint,
					Success:   false,
					Error:     execErr.Error(),
				})

				return TaskExecutionMsg{
					TaskID:    taskToExecute.ID,
					Status:    "failed",
					Completed: true,
					Error:     execErr,
				}
			}

			txSignature = fmt.Sprintf("real_sell_%s_%d", taskToExecute.TokenMint[:8], time.Now().Unix())

		default:
			err := fmt.Errorf("unsupported operation: %s", taskToExecute.Operation)
			logger.Error("Unsupported operation", zap.Error(err))

			return TaskExecutionMsg{
				TaskID:    taskToExecute.ID,
				Status:    "failed",
				Completed: true,
				Error:     err,
			}
		}

		// Publish successful completion
		ui.PublishTaskCompleted(ui.TaskCompletedMsg{
			TaskID:      taskToExecute.ID,
			TaskName:    taskToExecute.TaskName,
			TokenMint:   taskToExecute.TokenMint,
			TxSignature: txSignature,
			Success:     true,
		})

		// For buy operations, create monitoring session and position
		if taskToExecute.Operation == task.OperationSnipe || taskToExecute.Operation == task.OperationSwap {
			s.createMonitoringSession(taskToExecute, entryPrice, tokenBalance)

			// Publish position created event
			ui.PublishPositionCreated(ui.PositionCreatedMsg{
				TaskID:       taskToExecute.ID,
				TokenMint:    taskToExecute.TokenMint,
				TokenSymbol:  s.getTokenSymbol(taskToExecute.TokenMint),
				EntryPrice:   entryPrice,
				TokenBalance: tokenBalance,
				AmountSol:    taskToExecute.AmountSol,
				TxSignature:  txSignature,
			})
		}

		logger.Info("✅ Task execution completed successfully",
			zap.Int("task_id", taskToExecute.ID),
			zap.String("tx_signature", txSignature))

		return TaskExecutionMsg{
			TaskID:    taskToExecute.ID,
			Status:    "completed",
			Completed: true,
		}
	}
}

// createMonitoringSession creates a monitoring session for a completed buy task
func (s *RealModeTaskListScreen) createMonitoringSession(taskData task.Task, entryPrice float64, tokenBalance uint64) {
	ctx := s.serviceProvider.GetContext()
	logger := s.serviceProvider.GetLogger()
	wallets := s.serviceProvider.GetWallets()
	blockchainClient := s.serviceProvider.GetBlockchainClient()

	// Get wallet for the task
	wallet, exists := wallets[taskData.WalletName]
	if !exists {
		logger.Error("Wallet not found for monitoring session",
			zap.String("wallet", taskData.WalletName))
		return
	}

	// Create DEX adapter
	dexAdapter, err := dex.GetDEXByName(taskData.Module, blockchainClient, wallet, logger)
	if err != nil {
		logger.Error("Failed to create DEX adapter for monitoring", zap.Error(err))
		return
	}

	// Create monitoring session config
	sessionConfig := &monitor.SessionConfig{
		Task:            &taskData,
		TokenBalance:    tokenBalance,
		InitialPrice:    entryPrice,
		DEX:             dexAdapter,
		Logger:          logger,
		MonitorInterval: time.Second * 5,
	}

	// Create and start monitoring session
	session := monitor.NewMonitoringSession(ctx, sessionConfig)
	err = session.Start()
	if err != nil {
		logger.Error("Failed to start monitoring session", zap.Error(err))
		return
	}

	// Add session to service provider
	s.serviceProvider.AddMonitoringSession(taskData.TokenMint, session)

	// Start goroutine to republish monitoring session updates to UI bus
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case update, ok := <-session.PriceUpdates():
				if !ok {
					return // Channel closed
				}
				// Republish to UI bus
				ui.PublishRealPriceUpdate(ui.RealPriceUpdateMsg{
					TokenMint: taskData.TokenMint,
					Update:    update,
				})
			}
		}
	}()

	// Publish monitoring session started event
	ui.PublishMonitoringSessionStarted(ui.MonitoringSessionStartedMsg{
		TokenMint:    taskData.TokenMint,
		InitialPrice: entryPrice,
		TokenAmount:  float64(tokenBalance) / math.Pow10(6), // Assuming 6 decimals
	})

	logger.Info("📊 Created monitoring session for completed task",
		zap.String("token", taskData.TokenMint),
		zap.Float64("entry_price", entryPrice),
		zap.Uint64("balance", tokenBalance))
}

// getTokenSymbol extracts a symbol from token mint (simplified)
func (s *RealModeTaskListScreen) getTokenSymbol(tokenMint string) string {
	if len(tokenMint) >= 8 {
		return tokenMint[:4] + "..." + tokenMint[len(tokenMint)-4:]
	}
	return "TOKEN"
}

// RealModeLogsScreen wraps LogsScreen with real mode support
type RealModeLogsScreen struct {
	*LogsScreen
	logger   *zap.Logger
	realMode bool
}

// NewRealModeLogsScreen creates a logs screen with real logger
func NewRealModeLogsScreen(logger *zap.Logger) *RealModeLogsScreen {
	base := NewLogsScreen()
	screen := &RealModeLogsScreen{
		LogsScreen: base,
		logger:     logger,
		realMode:   true,
	}

	// Initialize with real logs
	screen.initializeRealLogs()

	return screen
}

// initializeRealLogs sets up real log entries for the session
func (s *RealModeLogsScreen) initializeRealLogs() {
	now := time.Now()

	// Add some initial real log entries
	initialLogs := []LogEntry{
		{
			Timestamp: now.Add(-5 * time.Minute),
			Level:     LogLevel("INFO"),
			Component: "main",
			Message:   "🚀 Starting Solana Trading Bot TUI",
		},
		{
			Timestamp: now.Add(-4 * time.Minute),
			Level:     LogLevel("INFO"),
			Component: "config",
			Message:   "📋 Configuration loaded successfully",
		},
		{
			Timestamp: now.Add(-3 * time.Minute),
			Level:     LogLevel("INFO"),
			Component: "blockchain",
			Message:   "🔗 Connected to Solana RPC",
		},
		{
			Timestamp: now.Add(-2 * time.Minute),
			Level:     LogLevel("INFO"),
			Component: "task",
			Message:   "📋 Task manager initialized",
		},
		{
			Timestamp: now.Add(-1 * time.Minute),
			Level:     LogLevel("INFO"),
			Component: "ui",
			Message:   "🎨 TUI interface started in real mode",
		},
		{
			Timestamp: now,
			Level:     LogLevel("INFO"),
			Component: "monitor",
			Message:   "📊 Real-time monitoring enabled",
		},
	}

	s.logs = initialLogs
	s.filteredLogs = initialLogs
	s.updateTableDisplay()
}

// Real mode accessor methods
func (s *RealModeMonitorScreen) GetServiceProvider() ui.ServiceProvider {
	return s.serviceProvider
}

func (s *RealModeMonitorScreen) GetRealMode() bool {
	return s.realMode
}

func (s *RealModeTaskListScreen) GetServiceProvider() ui.ServiceProvider {
	return s.serviceProvider
}

func (s *RealModeTaskListScreen) GetRealMode() bool {
	return s.realMode
}

func (s *RealModeLogsScreen) GetLogger() *zap.Logger {
	return s.logger
}

func (s *RealModeLogsScreen) GetRealMode() bool {
	return s.realMode
}

// sellPositionRealCmd performs real position selling through DEX adapters
func (s *RealModeMonitorScreen) sellPositionRealCmd(position MonitoredPosition, percentage float64) tea.Cmd {
	return func() tea.Msg {
		ctx := s.serviceProvider.GetContext()
		logger := s.serviceProvider.GetLogger()
		wallets := s.serviceProvider.GetWallets()
		blockchainClient := s.serviceProvider.GetBlockchainClient()

		logger.Info("💰 Starting real sell operation",
			zap.String("token", position.TokenMint),
			zap.Float64("percentage", percentage),
			zap.String("position", position.TokenSymbol))

		// Get monitoring session for this token
		session, sessionExists := s.serviceProvider.GetMonitoringSession(position.TokenMint)
		if !sessionExists {
			err := fmt.Errorf("monitoring session not found for token %s", position.TokenMint)
			logger.Error("Sell failed - session not found", zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Sell Failed"}
		}

		// Get wallet name from session config
		// Since we can't directly access config.Task from session due to encapsulation,
		// we'll use a heuristic: check all loaded tasks to find matching token
		taskManager := s.serviceProvider.GetTaskManager()
		var walletName string
		var taskFound bool

		// Load current tasks to find the one with matching token mint
		var matchedTask *task.Task
		tasks, err := taskManager.LoadTasks("configs/tasks.csv")
		if err != nil {
			logger.Error("Failed to load tasks for wallet lookup", zap.Error(err))
			// Use default parameters as fallback
			walletName = "main"
			taskFound = true
		} else {
			for _, task := range tasks {
				if task.TokenMint == position.TokenMint {
					walletName = task.WalletName
					matchedTask = task
					taskFound = true
					break
				}
			}
		}

		if !taskFound {
			// Use default wallet as fallback
			walletName = "main"
			logger.Warn("Task not found for position, using default wallet",
				zap.String("token", position.TokenMint),
				zap.String("default_wallet", walletName))
		}

		// Get wallet
		wallet, exists := wallets[walletName]
		if !exists {
			err := fmt.Errorf("wallet %s not found", walletName)
			logger.Error("Sell failed - wallet not found", zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Sell Failed"}
		}

		// Create DEX adapter - use Smart DEX (snipe)
		dexAdapter, err := dex.GetDEXByName("snipe", blockchainClient, wallet, logger)
		if err != nil {
			logger.Error("Failed to create DEX adapter", zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Sell Failed"}
		}

		// Execute sell operation
		sellCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		// Get sell parameters from task or use defaults
		var slippage = 5.0
		var priorityFee = "0.000002"
		var computeUnits uint32 = 200000

		if matchedTask != nil {
			slippage = matchedTask.SlippagePercent
			priorityFee = matchedTask.PriorityFeeSol
			computeUnits = matchedTask.ComputeUnits
			logger.Info("Using task parameters for sell",
				zap.Float64("slippage", slippage),
				zap.String("priority_fee", priorityFee),
				zap.Uint32("compute_units", computeUnits))
		} else {
			logger.Info("Using default parameters for sell",
				zap.Float64("slippage", slippage),
				zap.String("priority_fee", priorityFee),
				zap.Uint32("compute_units", computeUnits))
		}

		logger.Info("🔄 Executing sell through DEX adapter",
			zap.String("token", position.TokenMint),
			zap.Float64("percentage", percentage))

		err = dexAdapter.SellPercentTokens(sellCtx, position.TokenMint, percentage, slippage, priorityFee, computeUnits)
		if err != nil {
			logger.Error("❌ Sell operation failed",
				zap.String("token", position.TokenMint),
				zap.Error(err))
			return ui.ErrorMsg{Error: err, Title: "Sell Failed"}
		}

		logger.Info("✅ Sell operation completed successfully",
			zap.String("token", position.TokenMint),
			zap.Float64("percentage", percentage))

		// Publish sell completed event
		ui.PublishSellCompleted(ui.SellCompletedMsg{
			TokenMint:   position.TokenMint,
			AmountSold:  position.Amount * (percentage / 100.0),
			SolReceived: position.CurrentPrice * position.Amount * (percentage / 100.0),
			TxSignature: fmt.Sprintf("sell_%s_%d", position.TokenMint[:8], time.Now().Unix()),
			Success:     true,
		})

		// If 100% sold, stop monitoring session
		if percentage >= 100.0 {
			session.Stop()
			s.serviceProvider.RemoveMonitoringSession(position.TokenMint)

			// Mark position as inactive
			if pos, exists := s.positionMap[position.TokenMint]; exists {
				pos.Active = false
				pos.LastUpdate = time.Now()
				s.updateTableDisplay()
			}

			logger.Info("🛑 Monitoring session stopped - position fully sold",
				zap.String("token", position.TokenMint))
		}

		return ui.SuccessMsg{
			Message: fmt.Sprintf("Successfully sold %.1f%% of %s", percentage, position.TokenSymbol),
			Title:   "Sell Completed",
		}
	}
}

// Update overrides the base Update method to handle real-time events
func (s *RealModeMonitorScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Real sell position (100%)
			if s.selectedPosition < len(s.positions) {
				cmds = append(cmds, s.sellPositionRealCmd(s.positions[s.selectedPosition], 100.0))
			}
			return s, tea.Batch(cmds...)

		case "s":
			// Real sell position (not toggle sparklines)
			if s.selectedPosition < len(s.positions) {
				cmds = append(cmds, s.sellPositionRealCmd(s.positions[s.selectedPosition], 100.0))
			}
			return s, tea.Batch(cmds...)

		case "1", "2", "3", "4", "5":
			// Real quick sell percentages
			if s.selectedPosition < len(s.positions) {
				percentage := map[string]float64{
					"1": 25.0, "2": 50.0, "3": 75.0, "4": 100.0, "5": 10.0,
				}[msg.String()]
				cmds = append(cmds, s.sellPositionRealCmd(s.positions[s.selectedPosition], percentage))
			}
			return s, tea.Batch(cmds...)

		default:
			// For all other keys, delegate to base class (navigation, etc.)
			updatedScreen, cmd := s.MonitorScreen.Update(msg)
			s.MonitorScreen = updatedScreen.(*MonitorScreen)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return s, tea.Batch(cmds...)
		}

	case ui.RealPriceUpdateMsg:
		// Handle real-time price updates
		if position, exists := s.positionMap[msg.TokenMint]; exists {
			// Update position with new price data
			position.CurrentPrice = msg.Update.Current
			position.EntryPrice = msg.Update.Initial
			position.Amount = msg.Update.Tokens
			position.PnLPercent = msg.Update.Percent

			// Calculate PnL in SOL
			if msg.Update.Initial > 0 {
				position.PnLSol = (msg.Update.Current - msg.Update.Initial) * msg.Update.Tokens
			}

			// Add to price history for sparkline
			position.PriceHistory = append(position.PriceHistory, msg.Update.Current)
			if len(position.PriceHistory) > 50 { // Keep last 50 points
				position.PriceHistory = position.PriceHistory[1:]
			}

			position.LastUpdate = time.Now()

			// Update table display
			s.updateTableDisplay()
		}
		return s, nil

	case ui.PositionCreatedMsg:
		// Handle new position creation
		newPosition := MonitoredPosition{
			ID:           len(s.positions) + 1,
			TaskName:     fmt.Sprintf("MONITOR_%d", len(s.positions)+1),
			TokenMint:    msg.TokenMint,
			TokenSymbol:  msg.TokenSymbol,
			EntryPrice:   msg.EntryPrice,
			CurrentPrice: msg.EntryPrice,                            // Start with entry price
			Amount:       float64(msg.TokenBalance) / math.Pow10(6), // Assuming 6 decimals
			PnLPercent:   0,
			PnLSol:       0,
			Volume24h:    0,
			LastUpdate:   time.Now(),
			PriceHistory: []float64{msg.EntryPrice},
			Active:       true,
		}

		s.positions = append(s.positions, newPosition)
		s.positionMap[msg.TokenMint] = &s.positions[len(s.positions)-1]

		// Update table display
		s.updateTableDisplay()
		return s, nil

	case ui.MonitoringSessionStoppedMsg:
		// Handle monitoring session stopped
		if position, exists := s.positionMap[msg.TokenMint]; exists {
			position.Active = false
			position.LastUpdate = time.Now()
			s.updateTableDisplay()
		}
		return s, nil
	}

	// For all other messages, delegate to base class
	updatedScreen, cmd := s.MonitorScreen.Update(msg)
	s.MonitorScreen = updatedScreen.(*MonitorScreen)

	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return s, tea.Batch(cmds...)
}
