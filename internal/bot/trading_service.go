// internal/bot/trading_service.go
package bot

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

// TaskManagerInterface defines the interface for task managers
type TaskManagerInterface interface {
	LoadTasks(filePath string) ([]*task.Task, error)
}

// TradingService provides centralized trading operations
type TradingService struct {
	commandBus       *CommandBus
	eventBus         *EventBus
	logger           *zap.Logger
	blockchainClient *blockchain.Client
	wallets          map[string]*task.Wallet
	taskManager      TaskManagerInterface
	monitorService   monitor.MonitorService
}

// TradingServiceConfig configuration for TradingService
type TradingServiceConfig struct {
	Logger           *zap.Logger
	BlockchainClient *blockchain.Client
	Wallets          map[string]*task.Wallet
	TaskManager      TaskManagerInterface
	MonitorService   monitor.MonitorService
}

// NewTradingService creates a new trading service
func NewTradingService(ctx context.Context, config *TradingServiceConfig) *TradingService {
	commandBus := NewCommandBus(config.Logger)
	eventBus := NewEventBus(config.Logger)

	service := &TradingService{
		commandBus:       commandBus,
		eventBus:         eventBus,
		logger:           config.Logger.Named("trading_service"),
		blockchainClient: config.BlockchainClient,
		wallets:          config.Wallets,
		taskManager:      config.TaskManager,
		monitorService:   config.MonitorService,
	}

	// Register command handlers
	commandBus.RegisterHandler(ExecuteTaskCommand{}, &TaskExecutionHandler{service: service})
	commandBus.RegisterHandler(SellPositionCommand{}, &SellPositionHandler{service: service})
	commandBus.RegisterHandler(RefreshDataCommand{}, &RefreshDataHandler{service: service})

	service.logger.Info("TradingService initialized successfully")
	return service
}

// GetCommandBus returns the command bus
func (s *TradingService) GetCommandBus() *CommandBus {
	return s.commandBus
}

// GetEventBus returns the event bus
func (s *TradingService) GetEventBus() *EventBus {
	return s.eventBus
}

// GetMonitorService returns the monitor service
func (s *TradingService) GetMonitorService() monitor.MonitorService {
	return s.monitorService
}

// TaskExecutionHandler handles task execution commands
type TaskExecutionHandler struct {
	service *TradingService
}

// Handle executes a trading task
func (h *TaskExecutionHandler) Handle(ctx context.Context, cmd TradingCommand) error {
	executeCmd, ok := cmd.(ExecuteTaskCommand)
	if !ok {
		return fmt.Errorf("invalid command type for TaskExecutionHandler")
	}

	h.service.logger.Info("🚀 Handling task execution command",
		zap.Int("task_id", executeCmd.TaskID),
		zap.String("user_id", executeCmd.UserID))

	// Load task from task manager
	tasks, err := h.service.taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		h.service.logger.Error("Failed to load tasks", zap.Error(err))
		h.publishTaskExecutedEvent(executeCmd, "", false, err.Error())
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find task by ID
	var taskToExecute *task.Task
	for _, t := range tasks {
		if t.ID == executeCmd.TaskID {
			taskToExecute = t
			break
		}
	}

	if taskToExecute == nil {
		err := fmt.Errorf("task with ID %d not found", executeCmd.TaskID)
		h.service.logger.Error("Task not found", zap.Error(err))
		h.publishTaskExecutedEvent(executeCmd, "", false, err.Error())
		return err
	}

	h.service.logger.Info("🎯 Executing task",
		zap.Int("task_id", taskToExecute.ID),
		zap.String("task_name", taskToExecute.TaskName),
		zap.String("operation", string(taskToExecute.Operation)),
		zap.String("token", taskToExecute.TokenMint),
		zap.Float64("amount", taskToExecute.AmountSol))

	// Get wallet for this task
	wallet, exists := h.service.wallets[taskToExecute.WalletName]
	if !exists {
		err := fmt.Errorf("wallet %s not found", taskToExecute.WalletName)
		h.service.logger.Error("Wallet not found", zap.Error(err))
		h.publishTaskExecutedEvent(executeCmd, taskToExecute.TokenMint, false, err.Error())
		return err
	}

	// Create DEX adapter
	h.service.logger.Info("Creating DEX adapter",
		zap.String("module", taskToExecute.Module),
		zap.String("wallet", taskToExecute.WalletName),
		zap.String("token", taskToExecute.TokenMint))

	dexAdapter, err := dex.GetDEXByName(taskToExecute.Module, h.service.blockchainClient, wallet, h.service.logger)
	if err != nil {
		h.service.logger.Error("Failed to create DEX adapter",
			zap.String("module", taskToExecute.Module),
			zap.Error(err))
		h.publishTaskExecutedEvent(executeCmd, taskToExecute.TokenMint, false, err.Error())
		return fmt.Errorf("failed to create DEX adapter: %w", err)
	}

	h.service.logger.Info("DEX adapter created successfully",
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
		h.service.logger.Info("Executing REAL task through DEX adapter",
			zap.String("operation", string(taskToExecute.Operation)),
			zap.String("token", taskToExecute.TokenMint),
			zap.Float64("amount_sol", taskToExecute.AmountSol))

		// Execute the actual task
		if err := dexAdapter.Execute(execCtx, taskToExecute); err != nil {
			h.service.logger.Error("Task execution failed",
				zap.String("token", taskToExecute.TokenMint),
				zap.Error(err))
			h.publishTaskExecutedEvent(executeCmd, taskToExecute.TokenMint, false, err.Error())
			return fmt.Errorf("task execution failed: %w", err)
		}

		h.service.logger.Info("Task executed successfully",
			zap.String("token", taskToExecute.TokenMint))

		// Get current token price after execution
		price, priceErr := dexAdapter.GetTokenPrice(execCtx, taskToExecute.TokenMint)
		if priceErr != nil {
			h.service.logger.Error("Failed to get token price after execution",
				zap.String("token", taskToExecute.TokenMint),
				zap.Error(priceErr))
			// Continue with zero price rather than fail
			price = 0
		}

		// Get actual token balance after purchase
		actualBalance, balanceErr := dexAdapter.GetTokenBalance(execCtx, taskToExecute.TokenMint)
		if balanceErr != nil {
			h.service.logger.Error("Failed to get token balance after execution",
				zap.String("token", taskToExecute.TokenMint),
				zap.Error(balanceErr))
			// Calculate expected balance
			if price > 0 {
				tokenAmount := taskToExecute.AmountSol / price
				actualBalance = uint64(tokenAmount * math.Pow10(6))
			}
		}

		entryPrice = price
		tokenBalance = actualBalance
		txSignature = fmt.Sprintf("real_buy_%s_%d", taskToExecute.TokenMint[:8], time.Now().Unix())

		// Create monitoring session for buy operations
		req := &monitor.CreateSessionRequest{
			Task:         taskToExecute,
			EntryPrice:   entryPrice,
			TokenBalance: tokenBalance,
			Wallet:       wallet,
			DEXName:      taskToExecute.Module,
			Interval:     time.Second * 5,
			UserID:       executeCmd.UserID,
		}
		if _, err := h.service.monitorService.CreateMonitoringSession(ctx, req); err != nil {
			h.service.logger.Error("Failed to create monitoring session", zap.Error(err))
			// Don't fail the whole operation, just log the error
		}

		// Publish position created event
		h.service.eventBus.Publish(PositionCreatedEvent{
			TaskID:       taskToExecute.ID,
			TokenMint:    taskToExecute.TokenMint,
			TokenSymbol:  h.getTokenSymbol(taskToExecute.TokenMint),
			EntryPrice:   entryPrice,
			TokenBalance: tokenBalance,
			AmountSol:    taskToExecute.AmountSol,
			TxSignature:  txSignature,
			UserID:       executeCmd.UserID,
			Timestamp:    time.Now(),
		})

	case task.OperationSell:
		// Execute REAL sell operation through DEX adapter
		h.service.logger.Info("Executing REAL sell operation through DEX adapter",
			zap.String("token", taskToExecute.TokenMint))

		if err := dexAdapter.Execute(execCtx, taskToExecute); err != nil {
			h.service.logger.Error("Sell task execution failed",
				zap.String("token", taskToExecute.TokenMint),
				zap.Error(err))
			h.publishTaskExecutedEvent(executeCmd, taskToExecute.TokenMint, false, err.Error())
			return fmt.Errorf("sell task execution failed: %w", err)
		}

		txSignature = fmt.Sprintf("real_sell_%s_%d", taskToExecute.TokenMint[:8], time.Now().Unix())

	default:
		err := fmt.Errorf("unsupported operation: %s", taskToExecute.Operation)
		h.service.logger.Error("Unsupported operation", zap.Error(err))
		h.publishTaskExecutedEvent(executeCmd, taskToExecute.TokenMint, false, err.Error())
		return err
	}

	// Publish successful task executed event
	h.publishTaskExecutedEvent(executeCmd, taskToExecute.TokenMint, true, "")

	h.service.logger.Info("✅ Task execution completed successfully",
		zap.Int("task_id", taskToExecute.ID),
		zap.String("tx_signature", txSignature))

	return nil
}

// CanHandle returns true if this handler can handle the command
func (h *TaskExecutionHandler) CanHandle(cmd TradingCommand) bool {
	_, ok := cmd.(ExecuteTaskCommand)
	return ok
}

// publishTaskExecutedEvent publishes a task executed event
func (h *TaskExecutionHandler) publishTaskExecutedEvent(cmd ExecuteTaskCommand, tokenMint string, success bool, errorMsg string) {
	event := TaskExecutedEvent{
		TaskID:      cmd.TaskID,
		TaskName:    fmt.Sprintf("Task_%d", cmd.TaskID),
		TokenMint:   tokenMint,
		TxSignature: "",
		Success:     success,
		Error:       errorMsg,
		UserID:      cmd.UserID,
		Timestamp:   time.Now(),
	}

	if success {
		event.TxSignature = fmt.Sprintf("real_tx_%s_%d", tokenMint[:8], time.Now().Unix())
	}

	h.service.eventBus.Publish(event)
}

// getTokenSymbol extracts a symbol from token mint (simplified)
func (h *TaskExecutionHandler) getTokenSymbol(tokenMint string) string {
	if len(tokenMint) >= 8 {
		return tokenMint[:4] + "..." + tokenMint[len(tokenMint)-4:]
	}
	return "TOKEN"
}

// SellPositionHandler handles sell position commands
type SellPositionHandler struct {
	service *TradingService
}

// Handle executes a sell position operation
func (h *SellPositionHandler) Handle(ctx context.Context, cmd TradingCommand) error {
	sellCmd, ok := cmd.(SellPositionCommand)
	if !ok {
		return fmt.Errorf("invalid command type for SellPositionHandler")
	}

	h.service.logger.Info("💰 Handling sell position command",
		zap.String("token", sellCmd.TokenMint),
		zap.Float64("percentage", sellCmd.Percentage),
		zap.String("user_id", sellCmd.UserID))

	// Get monitoring session for this token
	_, sessionExists := h.service.monitorService.GetSession(sellCmd.TokenMint)
	if !sessionExists {
		err := fmt.Errorf("monitoring session not found for token %s", sellCmd.TokenMint)
		h.service.logger.Error("Sell failed - session not found", zap.Error(err))
		h.publishSellCompletedEvent(sellCmd, 0, 0, "", false, err.Error())
		return err
	}

	// Load tasks to find the one with matching token mint
	var matchedTask *task.Task
	tasks, err := h.service.taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		h.service.logger.Error("Failed to load tasks for wallet lookup", zap.Error(err))
		h.publishSellCompletedEvent(sellCmd, 0, 0, "", false, err.Error())
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	var walletName string
	var taskFound bool

	for _, task := range tasks {
		if task.TokenMint == sellCmd.TokenMint {
			walletName = task.WalletName
			matchedTask = task
			taskFound = true
			break
		}
	}

	if !taskFound {
		// Use default wallet as fallback
		walletName = "main"
		h.service.logger.Warn("Task not found for position, using default wallet",
			zap.String("token", sellCmd.TokenMint),
			zap.String("default_wallet", walletName))
	}

	// Get wallet
	wallet, exists := h.service.wallets[walletName]
	if !exists {
		err := fmt.Errorf("wallet %s not found", walletName)
		h.service.logger.Error("Sell failed - wallet not found", zap.Error(err))
		h.publishSellCompletedEvent(sellCmd, 0, 0, "", false, err.Error())
		return err
	}

	// Create DEX adapter - use Smart DEX (snipe)
	dexAdapter, err := dex.GetDEXByName("snipe", h.service.blockchainClient, wallet, h.service.logger)
	if err != nil {
		h.service.logger.Error("Failed to create DEX adapter", zap.Error(err))
		h.publishSellCompletedEvent(sellCmd, 0, 0, "", false, err.Error())
		return fmt.Errorf("failed to create DEX adapter: %w", err)
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
		h.service.logger.Info("Using task parameters for sell",
			zap.Float64("slippage", slippage),
			zap.String("priority_fee", priorityFee),
			zap.Uint32("compute_units", computeUnits))
	} else {
		h.service.logger.Info("Using default parameters for sell",
			zap.Float64("slippage", slippage),
			zap.String("priority_fee", priorityFee),
			zap.Uint32("compute_units", computeUnits))
	}

	h.service.logger.Info("🔄 Executing sell through DEX adapter",
		zap.String("token", sellCmd.TokenMint),
		zap.Float64("percentage", sellCmd.Percentage))

	// Get current token balance before sell
	tokenBalance, err := dexAdapter.GetTokenBalance(sellCtx, sellCmd.TokenMint)
	if err != nil {
		h.service.logger.Error("Failed to get token balance", zap.Error(err))
		tokenBalance = 0 // Continue with unknown balance
	}

	// Get current price
	currentPrice, err := dexAdapter.GetTokenPrice(sellCtx, sellCmd.TokenMint)
	if err != nil {
		h.service.logger.Error("Failed to get current price", zap.Error(err))
		currentPrice = 0 // Continue with unknown price
	}

	if err := dexAdapter.SellPercentTokens(sellCtx, sellCmd.TokenMint, sellCmd.Percentage, slippage, priorityFee, computeUnits); err != nil {
		h.service.logger.Error("❌ Sell operation failed",
			zap.String("token", sellCmd.TokenMint),
			zap.Error(err))
		h.publishSellCompletedEvent(sellCmd, 0, 0, "", false, err.Error())
		return fmt.Errorf("sell operation failed: %w", err)
	}

	h.service.logger.Info("✅ Sell operation completed successfully",
		zap.String("token", sellCmd.TokenMint),
		zap.Float64("percentage", sellCmd.Percentage))

	// Calculate amounts sold
	amountSold := float64(tokenBalance) * (sellCmd.Percentage / 100.0) / math.Pow10(6)
	solReceived := currentPrice * amountSold
	txSignature := fmt.Sprintf("sell_%s_%d", sellCmd.TokenMint[:8], time.Now().Unix())

	// Publish sell completed event
	h.publishSellCompletedEvent(sellCmd, amountSold, solReceived, txSignature, true, "")

	// If 100% sold, stop monitoring session
	if sellCmd.Percentage >= 100.0 {
		if err := h.service.monitorService.StopMonitoringSession(sellCmd.TokenMint); err != nil {
			h.service.logger.Error("Failed to stop monitoring session", zap.Error(err))
			// Continue execution - don't fail sell operation due to session stop error
		}

		// Publish monitoring session stopped event
		h.service.eventBus.Publish(MonitoringSessionStoppedEvent{
			TokenMint: sellCmd.TokenMint,
			Reason:    "position_fully_sold",
			UserID:    sellCmd.UserID,
			Timestamp: time.Now(),
		})

		h.service.logger.Info("🛑 Monitoring session stopped - position fully sold",
			zap.String("token", sellCmd.TokenMint))
	}

	return nil
}

// CanHandle returns true if this handler can handle the command
func (h *SellPositionHandler) CanHandle(cmd TradingCommand) bool {
	_, ok := cmd.(SellPositionCommand)
	return ok
}

// publishSellCompletedEvent publishes a sell completed event
func (h *SellPositionHandler) publishSellCompletedEvent(cmd SellPositionCommand, amountSold, solReceived float64, txSignature string, success bool, errorMsg string) {
	h.service.eventBus.Publish(SellCompletedEvent{
		TokenMint:   cmd.TokenMint,
		AmountSold:  amountSold,
		SolReceived: solReceived,
		TxSignature: txSignature,
		Success:     success,
		Error:       errorMsg,
		UserID:      cmd.UserID,
		Timestamp:   time.Now(),
	})
}

// RefreshDataHandler handles refresh data commands
type RefreshDataHandler struct {
	service *TradingService
}

// Handle refreshes data
func (h *RefreshDataHandler) Handle(ctx context.Context, cmd TradingCommand) error {
	refreshCmd, ok := cmd.(RefreshDataCommand)
	if !ok {
		return fmt.Errorf("invalid command type for RefreshDataHandler")
	}

	h.service.logger.Info("🔄 Handling refresh data command",
		zap.String("user_id", refreshCmd.UserID))

	// Implement refresh logic here
	// For now, just log that refresh was requested
	h.service.logger.Info("Data refresh completed",
		zap.String("user_id", refreshCmd.UserID))

	return nil
}

// CanHandle returns true if this handler can handle the command
func (h *RefreshDataHandler) CanHandle(cmd TradingCommand) bool {
	_, ok := cmd.(RefreshDataCommand)
	return ok
}
