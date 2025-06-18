package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/bot"
	"github.com/rovshanmuradov/solana-bot/internal/logger"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/router"
	"github.com/rovshanmuradov/solana-bot/internal/ui/screen"
	"go.uber.org/zap"
)

// AppModel represents the main TUI application model
type AppModel struct {
	router *router.Router
	width  int
	height int

	// Unified bot service
	botService *bot.BotService

	// Active monitoring sessions (legacy - deprecated)
	activeSessions map[string]*monitor.MonitoringSession
	sessionsMutex  sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewAppModel creates a new application model
func NewAppModel(ctx context.Context, cfg *task.Config, logger *zap.Logger) *AppModel {
	appCtx, cancel := context.WithCancel(ctx)

	// Create unified bot service
	botService, err := bot.NewBotService(appCtx, &bot.BotServiceConfig{
		Config: cfg,
		Logger: logger,
	})
	if err != nil {
		logger.Fatal("Failed to create bot service", zap.Error(err))
	}

	// Start the bot service
	if err := botService.Start(); err != nil {
		logger.Fatal("Failed to start bot service", zap.Error(err))
	}

	// Create the main menu screen as the initial screen
	mainMenu := screen.NewMainMenuScreen()

	// Initialize the router with the main menu
	r := router.New(mainMenu)

	app := &AppModel{
		router:         r,
		botService:     botService,
		activeSessions: make(map[string]*monitor.MonitoringSession),
		ctx:            appCtx,
		cancel:         cancel,
	}

	return app
}

// Init initializes the application
func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.router.Init(),
		ui.ListenBus(), // Start listening to the event bus
		tea.EnterAltScreen,
	)
}

// Update handles application-level updates
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.router.SetSize(msg.Width, msg.Height)

		// Forward size message to router
		updatedRouter, cmd := m.router.Update(msg)
		m.router = updatedRouter.(*router.Router)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		default:
			// Forward to router
			updatedRouter, cmd := m.router.Update(msg)
			m.router = updatedRouter.(*router.Router)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case ui.RouterMsg:
		// Handle navigation requests
		cmd := m.handleNavigation(msg.To)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	default:
		// Forward all other messages to router
		updatedRouter, cmd := m.router.Update(msg)
		m.router = updatedRouter.(*router.Router)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Continue listening for events
	cmds = append(cmds, ui.ListenBus())

	return m, tea.Batch(cmds...)
}

// handleNavigation handles navigation to different screens
func (m *AppModel) handleNavigation(route ui.Route) tea.Cmd {
	var newScreen router.Screen

	switch route {
	case ui.RouteMainMenu:
		newScreen = screen.NewMainMenuScreen()

	case ui.RouteCreateTask:
		newScreen = screen.NewCreateTaskWizard()

	case ui.RouteTaskList:
		// Pass real services to TaskListScreen
		newScreen = screen.NewRealModeTaskListScreen(m)

	case ui.RouteMonitor:
		// Pass real services to MonitorScreen
		newScreen = screen.NewRealModeMonitorScreen(m)

	case ui.RouteLogs:
		// Use existing logs screen for now
		newScreen = screen.NewLogsScreen()

	case ui.RouteSettings:
		// TODO: Create settings screen
		newScreen = screen.NewMainMenuScreen() // Fallback to main menu for now

	default:
		// Unknown route, stay on current screen
		return nil
	}

	// If we're going to main menu, replace the current screen
	// Otherwise, push the new screen onto the stack
	if route == ui.RouteMainMenu {
		return m.router.Replace(newScreen)
	} else {
		return m.router.Push(newScreen)
	}
}

// View renders the application
func (m *AppModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	return m.router.View()
}

// AddMonitoringSession adds a new monitoring session
func (m *AppModel) AddMonitoringSession(tokenMint string, session *monitor.MonitoringSession) {
	m.sessionsMutex.Lock()
	defer m.sessionsMutex.Unlock()
	m.activeSessions[tokenMint] = session
}

// GetMonitoringSession returns a monitoring session for a token
func (m *AppModel) GetMonitoringSession(tokenMint string) (*monitor.MonitoringSession, bool) {
	m.sessionsMutex.RLock()
	defer m.sessionsMutex.RUnlock()
	session, exists := m.activeSessions[tokenMint]
	return session, exists
}

// GetAllMonitoringSessions returns all active monitoring sessions
func (m *AppModel) GetAllMonitoringSessions() map[string]*monitor.MonitoringSession {
	m.sessionsMutex.RLock()
	defer m.sessionsMutex.RUnlock()

	// Return a copy to avoid race conditions
	sessions := make(map[string]*monitor.MonitoringSession)
	for k, v := range m.activeSessions {
		sessions[k] = v
	}
	return sessions
}

// RemoveMonitoringSession removes a monitoring session
func (m *AppModel) RemoveMonitoringSession(tokenMint string) {
	m.sessionsMutex.Lock()
	defer m.sessionsMutex.Unlock()

	if session, exists := m.activeSessions[tokenMint]; exists {
		session.Stop()
		delete(m.activeSessions, tokenMint)
	}
}

// GetTaskManager returns the task manager
func (m *AppModel) GetTaskManager() *task.Manager {
	return m.botService.GetTaskManager()
}

// GetBlockchainClient returns the blockchain client
func (m *AppModel) GetBlockchainClient() *blockchain.Client {
	return m.botService.GetBlockchainClient()
}

// GetWallets returns the wallets map
func (m *AppModel) GetWallets() map[string]*task.Wallet {
	return m.botService.GetWallets()
}

// GetLogger returns the logger
func (m *AppModel) GetLogger() *zap.Logger {
	return m.botService.GetLogger()
}

// GetConfig returns the config
func (m *AppModel) GetConfig() *task.Config {
	return m.botService.GetConfig()
}

// GetContext returns the app context
func (m *AppModel) GetContext() context.Context {
	return m.ctx
}

// GetCommandBus returns the command bus from bot service
func (m *AppModel) GetCommandBus() *bot.CommandBus {
	return m.botService.GetCommandBus()
}

// GetEventBus returns the event bus from bot service
func (m *AppModel) GetEventBus() *bot.EventBus {
	return m.botService.GetEventBus()
}

// GetTradingService returns the trading service
func (m *AppModel) GetTradingService() *bot.TradingService {
	return m.botService.GetTradingService()
}

// GetBotService returns the bot service
func (m *AppModel) GetBotService() *bot.BotService {
	return m.botService
}

// Shutdown gracefully shuts down the application
func (m *AppModel) Shutdown() {
	// Shutdown bot service (this will handle all service shutdowns)
	if m.botService != nil {
		if err := m.botService.Shutdown(m.ctx); err != nil {
			m.botService.GetLogger().Error("Failed to shutdown bot service", zap.Error(err))
		}
	}

	// Stop legacy monitoring sessions (deprecated)
	for _, session := range m.activeSessions {
		session.Stop()
	}

	// Cancel the context
	if m.cancel != nil {
		m.cancel()
	}
}

func main() {
	// Parse command line flags
	configPath := flag.String("config", "configs/config.json", "Path to config file")
	flag.Parse()

	// Create context with signal handling
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Load configuration
	cfg, err := task.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	appLogger, err := logger.CreatePrettyLogger(cfg.DebugLogging)
	if err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer func() {
		_ = appLogger.Sync()
	}()

	appLogger.Info("🚀 Starting Solana Trading Bot TUI")

	// Initialize the TUI program
	program := tea.NewProgram(
		NewAppModel(rootCtx, cfg, appLogger),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Start the program in a goroutine
	go func() {
		if _, err := program.Run(); err != nil {
			appLogger.Error("💥 TUI application failed", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	<-rootCtx.Done()

	appLogger.Info("🛑 Shutting down TUI application")
	program.Quit()
}
