package ui

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

// RecoveryHandler manages UI panic recovery
type RecoveryHandler struct {
	logger       *zap.Logger
	restartDelay time.Duration
	maxRestarts  int
	restartCount int
	mu           sync.Mutex
	program      *tea.Program
	createUI     func() (tea.Model, []tea.ProgramOption)
}

// NewRecoveryHandler creates a new recovery handler
func NewRecoveryHandler(logger *zap.Logger, createUI func() (tea.Model, []tea.ProgramOption)) *RecoveryHandler {
	return &RecoveryHandler{
		logger:       logger,
		restartDelay: 5 * time.Second,
		maxRestarts:  5,
		createUI:     createUI,
	}
}

// RunWithRecovery runs the UI with panic recovery
func (rh *RecoveryHandler) RunWithRecovery() error {
	for {
		err := rh.runUI()

		rh.mu.Lock()
		if err == nil {
			// Normal exit
			rh.mu.Unlock()
			return nil
		}

		rh.restartCount++
		if rh.restartCount > rh.maxRestarts {
			rh.mu.Unlock()
			return fmt.Errorf("UI crashed too many times (%d), giving up", rh.maxRestarts)
		}

		rh.logger.Error("UI crashed, will restart",
			zap.Error(err),
			zap.Int("restart_count", rh.restartCount),
			zap.Duration("delay", rh.restartDelay))

		rh.mu.Unlock()

		// Wait before restarting
		time.Sleep(rh.restartDelay)
	}
}

// runUI runs the UI with panic recovery
func (rh *RecoveryHandler) runUI() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("UI panic: %v\nStack: %s", r, debug.Stack())
			rh.logger.Error("UI panic recovered",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	model, opts := rh.createUI()
	rh.program = tea.NewProgram(model, opts...)

	// Run the UI
	if _, err := rh.program.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	return nil
}

// Stop gracefully stops the UI
func (rh *RecoveryHandler) Stop() {
	rh.mu.Lock()
	defer rh.mu.Unlock()

	if rh.program != nil {
		rh.program.Quit()
		rh.program = nil
	}
}

// GetRestartCount returns the number of restarts
func (rh *RecoveryHandler) GetRestartCount() int {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	return rh.restartCount
}

// UIManager manages the UI lifecycle with recovery
type UIManager struct {
	recovery   *RecoveryHandler
	logger     *zap.Logger
	isRunning  bool
	mu         sync.RWMutex
	shutdownCh chan struct{}
}

// NewUIManager creates a new UI manager
func NewUIManager(logger *zap.Logger, createUI func() (tea.Model, []tea.ProgramOption)) *UIManager {
	return &UIManager{
		recovery:   NewRecoveryHandler(logger, createUI),
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}
}

// Start starts the UI in a separate goroutine with recovery
func (um *UIManager) Start() error {
	um.mu.Lock()
	if um.isRunning {
		um.mu.Unlock()
		return fmt.Errorf("UI is already running")
	}
	um.isRunning = true
	um.mu.Unlock()

	go func() {
		defer func() {
			um.mu.Lock()
			um.isRunning = false
			um.mu.Unlock()
		}()

		err := um.recovery.RunWithRecovery()
		if err != nil {
			um.logger.Error("UI manager stopped with error", zap.Error(err))
		} else {
			um.logger.Info("UI manager stopped normally")
		}
	}()

	return nil
}

// Stop gracefully stops the UI
func (um *UIManager) Stop() {
	um.mu.Lock()
	defer um.mu.Unlock()

	if !um.isRunning {
		return
	}

	um.recovery.Stop()
	// Don't close shutdownCh if it might already be closed
	select {
	case <-um.shutdownCh:
		// Already closed
	default:
		close(um.shutdownCh)
	}
	um.isRunning = false
}

// IsRunning returns whether the UI is currently running
func (um *UIManager) IsRunning() bool {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.isRunning
}

// GetRestartCount returns the number of UI restarts
func (um *UIManager) GetRestartCount() int {
	return um.recovery.GetRestartCount()
}

// SafeUIWrapper wraps UI operations with panic recovery
type SafeUIWrapper struct {
	model  tea.Model
	logger *zap.Logger
}

// NewSafeUIWrapper creates a new safe UI wrapper
func NewSafeUIWrapper(model tea.Model, logger *zap.Logger) *SafeUIWrapper {
	return &SafeUIWrapper{
		model:  model,
		logger: logger,
	}
}

// Init wraps the Init method with panic recovery
func (sw *SafeUIWrapper) Init() (cmd tea.Cmd) {
	defer sw.recoverFromPanic("Init", &cmd)
	return sw.model.Init()
}

// Update wraps the Update method with panic recovery
func (sw *SafeUIWrapper) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	defer sw.recoverFromPanic("Update", &cmd)
	model, cmd = sw.model.Update(msg)
	return sw, cmd
}

// View wraps the View method with panic recovery
func (sw *SafeUIWrapper) View() (view string) {
	defer func() {
		if r := recover(); r != nil {
			sw.logger.Error("View panic recovered",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
			view = "UI Error: View crashed. Press Ctrl+C to exit."
		}
	}()
	return sw.model.View()
}

// recoverFromPanic recovers from panics in UI methods
func (sw *SafeUIWrapper) recoverFromPanic(method string, cmd *tea.Cmd) {
	if r := recover(); r != nil {
		sw.logger.Error("UI method panic recovered",
			zap.String("method", method),
			zap.Any("panic", r),
			zap.String("stack", string(debug.Stack())))
		// Return a nil command to prevent further issues
		*cmd = nil
	}
}
