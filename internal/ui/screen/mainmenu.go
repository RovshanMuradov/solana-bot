package screen

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/domain"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/component"
	"github.com/rovshanmuradov/solana-bot/internal/ui/router"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// MenuItem represents a menu item
type MenuItem struct {
	Label       string
	Description string
	Route       ui.Route
	Enabled     bool
}

// MainMenuScreen represents the main menu screen
type MainMenuScreen struct {
	width  int
	height int
	keyMap ui.KeyMap

	// UI components
	helpBar *component.HelpBar

	// State
	selectedIndex int
	menuItems     []MenuItem

	// Styling
	titleStyle       lipgloss.Style
	menuItemStyle    lipgloss.Style
	selectedStyle    lipgloss.Style
	descriptionStyle lipgloss.Style
	headerStyle      lipgloss.Style

	// Status info
	walletInfo string
	lastUpdate time.Time
}

// NewMainMenuScreen creates a new main menu screen
func NewMainMenuScreen() *MainMenuScreen {
	palette := style.DefaultPalette()
	keyMap := ui.DefaultKeyMap()

	// Define menu items
	menuItems := []MenuItem{
		{
			Label:       "▶ Launch Tasks",
			Description: "Run existing tasks from your task list",
			Route:       ui.RouteTaskList,
			Enabled:     true,
		},
		{
			Label:       "✏ Create Task",
			Description: "Create a new trading task",
			Route:       ui.RouteCreateTask,
			Enabled:     true,
		},
		{
			Label:       "📊 Monitor",
			Description: "Monitor active trading positions",
			Route:       ui.RouteMonitor,
			Enabled:     true,
		},
		{
			Label:       "⚙ Settings",
			Description: "Configure application settings",
			Route:       ui.RouteSettings,
			Enabled:     true,
		},
		{
			Label:       "📜 Logs",
			Description: "View application logs and activity",
			Route:       ui.RouteLogs,
			Enabled:     true,
		},
	}

	helpBar := component.NewHelpBar().
		SetKeyBindings(keyMap.ContextualHelp(ui.RouteMainMenu)).
		SetCompact(false)

	return &MainMenuScreen{
		keyMap:        keyMap,
		selectedIndex: 0,
		menuItems:     menuItems,
		helpBar:       helpBar,
		lastUpdate:    time.Now(),

		titleStyle: lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true).
			Margin(1, 0).
			Align(lipgloss.Center),

		menuItemStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(0, 2).
			Margin(0, 0, 1, 0),

		selectedStyle: lipgloss.NewStyle().
			Foreground(palette.Background).
			Background(palette.Primary).
			Padding(0, 2).
			Margin(0, 0, 1, 0).
			Bold(true),

		descriptionStyle: lipgloss.NewStyle().
			Foreground(palette.TextMuted).
			Padding(0, 4).
			Margin(0, 0, 1, 0).
			Italic(true),

		headerStyle: lipgloss.NewStyle().
			Foreground(palette.Secondary).
			Bold(true).
			Padding(0, 2),
	}
}

// Init initializes the main menu screen
func (m *MainMenuScreen) Init() tea.Cmd {
	return tea.Batch(
		ui.ListenBus(), // Listen for domain events
		tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tea.Msg(t) // For updating the clock
		}),
	)
}

// Update handles screen updates
func (m *MainMenuScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keyMap.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keyMap.Up):
			m.moveUp()

		case key.Matches(msg, m.keyMap.Down):
			m.moveDown()

		case key.Matches(msg, m.keyMap.Enter):
			if m.selectedIndex < len(m.menuItems) && m.menuItems[m.selectedIndex].Enabled {
				// Navigate to selected route
				route := m.menuItems[m.selectedIndex].Route
				cmds = append(cmds, func() tea.Msg {
					return ui.RouterMsg{To: route}
				})
			}

		// Direct shortcuts
		case key.Matches(msg, m.keyMap.CreateTask):
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteCreateTask}
			})

		case key.Matches(msg, m.keyMap.RunTasks):
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteTaskList}
			})

		case key.Matches(msg, m.keyMap.Monitor):
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteMonitor}
			})

		case key.Matches(msg, m.keyMap.Settings):
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteSettings}
			})

		case key.Matches(msg, m.keyMap.Logs):
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteLogs}
			})
		}

	case time.Time:
		m.lastUpdate = msg
		// Schedule next update
		cmds = append(cmds, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tea.Msg(t)
		}))

	case ui.DomainEventMsg:
		// Handle domain events to update status information
		m.handleDomainEvent(msg.Event)

	case ui.ErrorMsg:
		// Show error status
		// TODO: Implement error display

	case ui.SuccessMsg:
		// Show success status
		// TODO: Implement success display
	}

	// Continue listening for events
	cmds = append(cmds, ui.ListenBus())

	return m, tea.Batch(cmds...)
}

// View renders the main menu screen
func (m *MainMenuScreen) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var content strings.Builder

	// Header with title and status
	header := m.renderHeader()
	content.WriteString(header)
	content.WriteString("\n\n")

	// Menu items
	menu := m.renderMenu()
	content.WriteString(menu)
	content.WriteString("\n")

	// Help bar
	help := m.helpBar.SetWidth(m.width).View()
	content.WriteString(help)

	// Center the content if there's enough space
	result := content.String()
	if m.width > 80 {
		result = lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			result)
	}

	return result
}

// SetSize sets the screen dimensions
func (m *MainMenuScreen) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.helpBar.SetWidth(width)
}

// renderHeader renders the screen header
func (m *MainMenuScreen) renderHeader() string {
	// Title
	title := "🚀 Solana Trading Bot"
	styledTitle := m.titleStyle.Width(m.width).Render(title)

	// Status line with current time and wallet info
	timeStr := m.lastUpdate.Format("15:04:05")
	walletStr := m.walletInfo
	if walletStr == "" {
		walletStr = "No wallet loaded"
	}

	statusLine := fmt.Sprintf("Time: %s • %s", timeStr, walletStr)
	styledStatus := m.headerStyle.Width(m.width).Align(lipgloss.Center).Render(statusLine)

	return lipgloss.JoinVertical(lipgloss.Center, styledTitle, styledStatus)
}

// renderMenu renders the menu items
func (m *MainMenuScreen) renderMenu() string {
	var menuItems []string

	for i, item := range m.menuItems {
		var itemStyle lipgloss.Style
		if i == m.selectedIndex {
			itemStyle = m.selectedStyle
		} else {
			itemStyle = m.menuItemStyle
		}

		// Disable styling for disabled items
		if !item.Enabled {
			palette := style.DefaultPalette()
			itemStyle = itemStyle.Foreground(palette.TextMuted)
		}

		styledItem := itemStyle.Render(item.Label)
		menuItems = append(menuItems, styledItem)

		// Add description for selected item
		if i == m.selectedIndex {
			description := m.descriptionStyle.Render(item.Description)
			menuItems = append(menuItems, description)
		}
	}

	menu := strings.Join(menuItems, "\n")

	// Add border around menu
	menuStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(style.DefaultPalette().Primary).
		Padding(2, 4).
		Margin(1, 0)

	return menuStyle.Render(menu)
}

// moveUp moves selection up
func (m *MainMenuScreen) moveUp() {
	if m.selectedIndex > 0 {
		m.selectedIndex--
	} else {
		m.selectedIndex = len(m.menuItems) - 1
	}
}

// moveDown moves selection down
func (m *MainMenuScreen) moveDown() {
	if m.selectedIndex < len(m.menuItems)-1 {
		m.selectedIndex++
	} else {
		m.selectedIndex = 0
	}
}

// handleDomainEvent processes domain events to update UI state
func (m *MainMenuScreen) handleDomainEvent(event domain.Event) {
	// TODO: Handle domain events to update status information
	// For example, update wallet info, task counts, etc.
}

// SetWalletInfo sets the wallet information to display
func (m *MainMenuScreen) SetWalletInfo(info string) {
	m.walletInfo = info
}

// GetSelectedRoute returns the currently selected route
func (m *MainMenuScreen) GetSelectedRoute() ui.Route {
	if m.selectedIndex < len(m.menuItems) {
		return m.menuItems[m.selectedIndex].Route
	}
	return ui.RouteMainMenu
}
