package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/logger"
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
}

// NewAppModel creates a new application model
func NewAppModel() *AppModel {
	// Create the main menu screen as the initial screen
	mainMenu := screen.NewMainMenuScreen()

	// Initialize the router with the main menu
	r := router.New(mainMenu)

	return &AppModel{
		router: r,
	}
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
		newScreen = screen.NewTaskListScreen()

	case ui.RouteMonitor:
		newScreen = screen.NewMonitorScreen()

	case ui.RouteLogs:
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
		NewAppModel(),
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
