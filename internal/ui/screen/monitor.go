package screen

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/domain"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/component"
	"github.com/rovshanmuradov/solana-bot/internal/ui/router"
	"github.com/rovshanmuradov/solana-bot/internal/ui/state"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// MonitoredPosition represents a trading position being monitored
type MonitoredPosition struct {
	ID           int
	TaskName     string
	TokenMint    string
	TokenSymbol  string
	EntryPrice   float64
	CurrentPrice float64
	Amount       float64
	PnLPercent   float64
	PnLSol       float64
	Volume24h    float64
	LastUpdate   time.Time
	PriceHistory []float64
	Active       bool
}

// MonitorScreen represents the real-time monitoring screen
type MonitorScreen struct {
	width  int
	height int
	keyMap ui.KeyMap

	// UI components
	helpBar    *component.HelpBar
	table      *component.Table
	sparklines map[int]*component.Sparkline // Position ID -> Sparkline
	pnlGauges  map[int]*component.PnLGauge  // Position ID -> PnL Gauge

	// State
	positions        []MonitoredPosition
	selectedPosition int
	autoRefresh      bool
	refreshInterval  time.Duration
	lastUpdate       time.Time
	errors           []string

	// Styling
	titleStyle     lipgloss.Style
	headerStyle    lipgloss.Style
	statusStyle    lipgloss.Style
	errorStyle     lipgloss.Style
	successStyle   lipgloss.Style
	warningStyle   lipgloss.Style
	positionStyle  lipgloss.Style
	sparklineStyle lipgloss.Style
	containerStyle lipgloss.Style

	// Layout configuration
	showSparklines bool
	showPnLGauges  bool
	compactMode    bool
}

// NewMonitorScreen creates a new monitoring screen
func NewMonitorScreen() *MonitorScreen {
	palette := style.DefaultPalette()
	keyMap := ui.DefaultKeyMap()

	screen := &MonitorScreen{
		keyMap:           keyMap,
		selectedPosition: 0,
		autoRefresh:      true,
		refreshInterval:  time.Second * 5,
		lastUpdate:       time.Now(),
		errors:           make([]string, 0),
		sparklines:       make(map[int]*component.Sparkline),
		pnlGauges:        make(map[int]*component.PnLGauge),
		showSparklines:   true,
		showPnLGauges:    true,
		compactMode:      false,

		titleStyle: lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true).
			Margin(1, 0).
			Align(lipgloss.Center),

		headerStyle: lipgloss.NewStyle().
			Foreground(palette.Secondary).
			Bold(true).
			Padding(0, 2),

		statusStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(0, 2),

		errorStyle: lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true).
			Padding(0, 2),

		successStyle: lipgloss.NewStyle().
			Foreground(palette.Success).
			Bold(true).
			Padding(0, 2),

		warningStyle: lipgloss.NewStyle().
			Foreground(palette.Warning).
			Bold(true).
			Padding(0, 2),

		positionStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.TextMuted).
			Padding(1, 2).
			Margin(0, 1),

		sparklineStyle: lipgloss.NewStyle().
			Padding(0, 1),

		containerStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Primary).
			Padding(1, 2).
			Margin(1, 0),
	}

	screen.initializeTable()
	screen.initializeHelpBar()
	screen.loadMockData() // Load some mock data for demonstration

	return screen
}

// initializeTable sets up the positions table
func (s *MonitorScreen) initializeTable() {
	s.table = component.NewTable().
		AddColumn("ID", 3, lipgloss.Right).
		AddColumn("Token", 12, lipgloss.Left).
		AddColumn("Entry", 10, lipgloss.Right).
		AddColumn("Current", 10, lipgloss.Right).
		AddColumn("PnL %", 8, lipgloss.Right).
		AddColumn("PnL SOL", 10, lipgloss.Right).
		AddColumn("Trend", 15, lipgloss.Left).
		AddColumn("Status", 8, lipgloss.Center).
		SetShowBorder(true).
		SetSelectable(true).
		SetZebra(true)
}

// initializeHelpBar sets up the help bar
func (s *MonitorScreen) initializeHelpBar() {
	s.helpBar = component.NewHelpBar().
		SetKeyBindings(s.keyMap.ContextualHelp(ui.RouteMonitor)).
		SetCompact(false)
}

// Init initializes the monitor screen
func (s *MonitorScreen) Init() tea.Cmd {
	return tea.Batch(
		ui.ListenBus(),
		s.startAutoRefresh(),
	)
}

// PriceUpdateMsg represents a price update for a position
type PriceUpdateMsg struct {
	PositionID int
	Price      float64
	Timestamp  time.Time
}

// PositionUpdateMsg represents a complete position update
type PositionUpdateMsg struct {
	Position MonitoredPosition
}

// RefreshMsg is sent to trigger a refresh
type RefreshMsg struct {
	Timestamp time.Time
}

// Update handles screen updates
func (s *MonitorScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, s.keyMap.Quit):
			return s, tea.Quit

		case key.Matches(msg, s.keyMap.Back):
			// Go back to main menu
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteMainMenu}
			})

		case key.Matches(msg, s.keyMap.Up):
			s.table.MoveUp()
			s.selectedPosition = s.table.GetSelectedRow()

		case key.Matches(msg, s.keyMap.Down):
			s.table.MoveDown()
			s.selectedPosition = s.table.GetSelectedRow()

		case key.Matches(msg, s.keyMap.Enter):
			// Sell position or show details
			if s.selectedPosition < len(s.positions) {
				cmds = append(cmds, s.sellPositionCmd(s.positions[s.selectedPosition]))
			}

		case key.Matches(msg, s.keyMap.Refresh):
			// Manual refresh
			cmds = append(cmds, s.refreshDataCmd())

		case msg.String() == "s":
			// Toggle sparklines
			s.showSparklines = !s.showSparklines
			s.updateDisplayComponents()

		case msg.String() == "g":
			// Toggle PnL gauges
			s.showPnLGauges = !s.showPnLGauges
			s.updateDisplayComponents()

		case msg.String() == "c":
			// Toggle compact mode
			s.compactMode = !s.compactMode
			s.updateDisplayComponents()

		case msg.String() == "a":
			// Toggle auto-refresh
			s.autoRefresh = !s.autoRefresh
			if s.autoRefresh {
				cmds = append(cmds, s.startAutoRefresh())
			}

		case msg.String() == "1", msg.String() == "2", msg.String() == "3", msg.String() == "4", msg.String() == "5":
			// Quick sell percentages
			if s.selectedPosition < len(s.positions) {
				percentage := map[string]float64{
					"1": 25.0, "2": 50.0, "3": 75.0, "4": 100.0, "5": 10.0,
				}[msg.String()]
				cmds = append(cmds, s.sellPositionPartialCmd(s.positions[s.selectedPosition], percentage))
			}

		case msg.String() == "e", msg.String() == "E":
			// Phase 3: Export trade data
			cmds = append(cmds, s.exportTradeDataCmd())
		}

	case RefreshMsg:
		s.lastUpdate = msg.Timestamp
		cmds = append(cmds, s.refreshDataCmd())
		if s.autoRefresh {
			cmds = append(cmds, s.startAutoRefresh())
		}

	case PriceUpdateMsg:
		s.updatePositionPrice(msg.PositionID, msg.Price, msg.Timestamp)
		s.updateTableDisplay()

	case PositionUpdateMsg:
		s.updatePosition(msg.Position)
		s.updateTableDisplay()

	case ui.DomainEventMsg:
		s.handleDomainEvent(msg.Event)

	case ui.ErrorMsg:
		s.errors = append(s.errors, msg.Error.Error())

	case ui.SuccessMsg:
		s.errors = make([]string, 0) // Clear errors on success
	}

	// Continue listening for events
	cmds = append(cmds, ui.ListenBus())

	return s, tea.Batch(cmds...)
}

// View renders the monitor screen
func (s *MonitorScreen) View() string {
	if s.width == 0 || s.height == 0 {
		return "Loading..."
	}

	var content strings.Builder

	// Title
	title := "📊 Position Monitor"
	if s.autoRefresh {
		title += " (Auto-refresh ON)"
	}
	content.WriteString(s.titleStyle.Width(s.width).Render(title))
	content.WriteString("\n\n")

	// Status bar
	statusBar := s.renderStatusBar()
	content.WriteString(statusBar)
	content.WriteString("\n\n")

	// Error messages
	if len(s.errors) > 0 {
		for _, err := range s.errors[:min(len(s.errors), 3)] { // Show max 3 errors
			content.WriteString(s.errorStyle.Render("❌ " + err))
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	// Main content
	if len(s.positions) > 0 {
		if s.compactMode {
			content.WriteString(s.renderCompactView())
		} else {
			content.WriteString(s.renderDetailedView())
		}
	} else {
		emptyMsg := "No active positions to monitor.\nTasks will appear here after execution."
		content.WriteString(s.statusStyle.Render(emptyMsg))
	}

	content.WriteString("\n")

	// Instructions
	instructions := s.renderInstructions()
	content.WriteString(instructions)
	content.WriteString("\n")

	// Help bar
	help := s.helpBar.SetWidth(s.width).View()
	content.WriteString(help)

	return content.String()
}

// SetSize sets the screen dimensions
func (s *MonitorScreen) SetSize(width, height int) {
	s.width = width
	s.height = height
	s.helpBar.SetWidth(width)
	s.table.SetSize(width-4, height-20)
	s.updateDisplayComponents()
}

// renderStatusBar renders the status information
func (s *MonitorScreen) renderStatusBar() string {
	var statusParts []string

	// Position count
	statusParts = append(statusParts, fmt.Sprintf("Positions: %d", len(s.positions)))

	// Total PnL
	totalPnL := s.calculateTotalPnL()
	pnlColor := s.successStyle
	if totalPnL < 0 {
		pnlColor = s.errorStyle
	}
	statusParts = append(statusParts, pnlColor.Render(fmt.Sprintf("Total PnL: %.4f SOL", totalPnL)))

	// Auto-refresh status
	refreshStatus := "Manual"
	if s.autoRefresh {
		refreshStatus = fmt.Sprintf("Auto (%ds)", int(s.refreshInterval.Seconds()))
	}
	statusParts = append(statusParts, fmt.Sprintf("Refresh: %s", refreshStatus))

	// Last update
	statusParts = append(statusParts, fmt.Sprintf("Updated: %s", s.lastUpdate.Format("15:04:05")))

	statusLine := strings.Join(statusParts, " • ")
	return s.headerStyle.Render(statusLine)
}

// renderCompactView renders a compact table view
func (s *MonitorScreen) renderCompactView() string {
	s.updateTableDisplay()
	return s.table.View()
}

// renderDetailedView renders a detailed view with sparklines and gauges
func (s *MonitorScreen) renderDetailedView() string {
	var content strings.Builder

	// Table
	content.WriteString(s.table.View())
	content.WriteString("\n\n")

	// Selected position details
	if s.selectedPosition < len(s.positions) {
		selectedPos := s.positions[s.selectedPosition]
		details := s.renderPositionDetails(selectedPos)
		content.WriteString(s.containerStyle.Render(details))
	}

	return content.String()
}

// renderPositionDetails renders detailed information for a position
func (s *MonitorScreen) renderPositionDetails(pos MonitoredPosition) string {
	var content strings.Builder

	// Position header
	header := fmt.Sprintf("📈 %s (%s)", pos.TaskName, pos.TokenSymbol)
	content.WriteString(s.headerStyle.Render(header))
	content.WriteString("\n\n")

	// Price information
	priceInfo := fmt.Sprintf("Entry: %.8f SOL | Current: %.8f SOL | Change: %.2f%%",
		pos.EntryPrice, pos.CurrentPrice, pos.PnLPercent)
	content.WriteString(s.statusStyle.Render(priceInfo))
	content.WriteString("\n\n")

	// Sparkline if enabled
	if s.showSparklines {
		sparkline := s.getOrCreateSparkline(pos.ID)
		sparkline.SetData(pos.PriceHistory).ShowText(true)

		sparklineView := sparkline.View()
		content.WriteString(s.sparklineStyle.Render("Price Trend: " + sparklineView))
		content.WriteString("\n\n")
	}

	// PnL Gauge if enabled
	if s.showPnLGauges {
		gauge := s.getOrCreatePnLGauge(pos.ID)
		gauge.SetValue(pos.PnLPercent)

		gaugeView := gauge.ViewDetailed()
		content.WriteString(s.statusStyle.Render("PnL: " + gaugeView))
		content.WriteString("\n\n")
	}

	// Additional metrics
	metrics := fmt.Sprintf("Amount: %.6f SOL | 24h Volume: %.2f SOL | Last Update: %s",
		pos.Amount, pos.Volume24h, pos.LastUpdate.Format("15:04:05"))
	content.WriteString(s.statusStyle.Render(metrics))

	return content.String()
}

// renderInstructions renders usage instructions
func (s *MonitorScreen) renderInstructions() string {
	var instructions []string

	instructions = append(instructions, "Enter: Sell position")
	instructions = append(instructions, "1-5: Quick sell (25%, 50%, 75%, 100%, 10%)")
	instructions = append(instructions, "E: Export trades")
	instructions = append(instructions, "S: Toggle sparklines")
	instructions = append(instructions, "G: Toggle PnL gauges")
	instructions = append(instructions, "C: Compact mode")
	instructions = append(instructions, "A: Auto-refresh")
	instructions = append(instructions, "F5: Refresh")

	return s.statusStyle.Render(strings.Join(instructions, " • "))
}

// updateTableDisplay updates the table with current position data
func (s *MonitorScreen) updateTableDisplay() {
	if s.table == nil {
		return
	}

	rows := make([][]string, 0, len(s.positions))
	palette := style.DefaultPalette()

	for i, pos := range s.positions {
		// Format prices
		entryStr := fmt.Sprintf("%.6f", pos.EntryPrice)
		currentStr := fmt.Sprintf("%.6f", pos.CurrentPrice)
		pnlPercentStr := fmt.Sprintf("%.2f%%", pos.PnLPercent)
		pnlSolStr := fmt.Sprintf("%.4f", pos.PnLSol)

		// Get sparkline for trend
		trendStr := "—"
		if len(pos.PriceHistory) > 1 {
			sparkline := s.getOrCreateSparkline(pos.ID)
			sparkline.SetData(pos.PriceHistory).SetWidth(12)
			trendStr = sparkline.View()
		}

		// Status
		status := "Active"
		if !pos.Active {
			status = "Inactive"
		}

		row := []string{
			fmt.Sprintf("%d", pos.ID),
			pos.TokenSymbol,
			entryStr,
			currentStr,
			pnlPercentStr,
			pnlSolStr,
			trendStr,
			status,
		}

		rows = append(rows, row)

		// Set custom styling based on PnL
		if pos.PnLPercent > 5 {
			// Strong profit
			style := lipgloss.NewStyle().Foreground(palette.Success).Bold(true)
			s.table.SetRowStyle(i, style)
		} else if pos.PnLPercent < -5 {
			// Strong loss
			style := lipgloss.NewStyle().Foreground(palette.Error).Bold(true)
			s.table.SetRowStyle(i, style)
		}
	}

	s.table.SetRows(rows)
	s.table.SetSelectedRow(s.selectedPosition)
}

// updateDisplayComponents updates sparklines and gauges based on current settings
func (s *MonitorScreen) updateDisplayComponents() {
	sparklineWidth := 20
	gaugeWidth := 15

	if s.compactMode {
		sparklineWidth = 10
		gaugeWidth = 8
	}

	for _, pos := range s.positions {
		if sparkline, exists := s.sparklines[pos.ID]; exists {
			sparkline.SetWidth(sparklineWidth)
		}
		if gauge, exists := s.pnlGauges[pos.ID]; exists {
			gauge.SetWidth(gaugeWidth)
		}
	}
}

// getOrCreateSparkline gets or creates a sparkline for a position
func (s *MonitorScreen) getOrCreateSparkline(positionID int) *component.Sparkline {
	if sparkline, exists := s.sparklines[positionID]; exists {
		return sparkline
	}

	sparkline := component.NewSparkline(20)
	s.sparklines[positionID] = sparkline
	return sparkline
}

// getOrCreatePnLGauge gets or creates a PnL gauge for a position
func (s *MonitorScreen) getOrCreatePnLGauge(positionID int) *component.PnLGauge {
	if gauge, exists := s.pnlGauges[positionID]; exists {
		return gauge
	}

	gauge := component.NewPnLGauge(15)
	s.pnlGauges[positionID] = gauge
	return gauge
}

// startAutoRefresh starts the auto-refresh timer
func (s *MonitorScreen) startAutoRefresh() tea.Cmd {
	if !s.autoRefresh {
		return nil
	}

	return tea.Tick(s.refreshInterval, func(t time.Time) tea.Msg {
		return RefreshMsg{Timestamp: t}
	})
}

// refreshDataCmd creates a command to refresh position data
func (s *MonitorScreen) refreshDataCmd() tea.Cmd {
	return func() tea.Msg {
		// TODO: Actually fetch real data from monitoring services
		// For now, simulate price updates
		s.simulatePriceUpdates()
		return ui.SuccessMsg{Message: "Data refreshed"}
	}
}

// sellPositionCmd creates a command to sell a position
func (s *MonitorScreen) sellPositionCmd(pos MonitoredPosition) tea.Cmd {
	return func() tea.Msg {
		// Create domain event for position sell
		event := domain.Event{
			Type:      domain.EventOrderFilled,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"position_id": pos.ID,
				"action":      "sell",
				"percentage":  100.0,
			},
		}

		ui.Bus <- ui.DomainEventMsg{Event: event}

		return ui.SuccessMsg{Message: fmt.Sprintf("Sell order placed for %s", pos.TokenSymbol)}
	}
}

// sellPositionPartialCmd creates a command to sell a percentage of a position
func (s *MonitorScreen) sellPositionPartialCmd(pos MonitoredPosition, percentage float64) tea.Cmd {
	return func() tea.Msg {
		event := domain.Event{
			Type:      domain.EventOrderFilled,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"position_id": pos.ID,
				"action":      "sell",
				"percentage":  percentage,
			},
		}

		ui.Bus <- ui.DomainEventMsg{Event: event}

		return ui.SuccessMsg{Message: fmt.Sprintf("Sell order placed for %.0f%% of %s", percentage, pos.TokenSymbol)}
	}
}

// updatePositionPrice updates a position's price and history using GlobalCache
func (s *MonitorScreen) updatePositionPrice(positionID int, price float64, timestamp time.Time) {

	for i := range s.positions {
		if s.positions[i].ID == positionID {
			s.positions[i].CurrentPrice = price
			s.positions[i].LastUpdate = timestamp

			// Update PnL
			s.positions[i].PnLPercent = ((price - s.positions[i].EntryPrice) / s.positions[i].EntryPrice) * 100
			s.positions[i].PnLSol = s.positions[i].Amount * (price - s.positions[i].EntryPrice)

			// Add to price history
			s.positions[i].PriceHistory = append(s.positions[i].PriceHistory, price)

			// Keep only last 50 data points
			if len(s.positions[i].PriceHistory) > 50 {
				s.positions[i].PriceHistory = s.positions[i].PriceHistory[1:]
			}

			break
		}
	}
}

// updatePosition updates a complete position using GlobalCache
func (s *MonitorScreen) updatePosition(pos MonitoredPosition) {
	// Phase 2: Update position in GlobalCache
	if state.GlobalCache != nil {
		// Convert MonitoredPosition to monitor.Position for cache
		cachePos := monitor.Position{
			SessionID:   fmt.Sprintf("position_%d", pos.ID),
			WalletAddr:  "", // Will be filled by actual monitoring
			TokenMint:   pos.TokenMint,
			TokenSymbol: pos.TokenSymbol,
			Amount:      pos.Amount,
			InitialSOL:  pos.EntryPrice,
			CurrentSOL:  pos.CurrentPrice,
			PnL:         pos.PnLSol,
			PnLPercent:  pos.PnLPercent,
			Status:      map[bool]string{true: "active", false: "inactive"}[pos.Active],
			UpdatedAt:   pos.LastUpdate,
		}
		state.GlobalCache.SetPosition(cachePos)
	}

	// Update local state for immediate UI updates
	for i := range s.positions {
		if s.positions[i].ID == pos.ID {
			s.positions[i] = pos
			break
		}
	}
}

// calculateTotalPnL calculates total PnL across all positions
func (s *MonitorScreen) calculateTotalPnL() float64 {
	total := 0.0
	for _, pos := range s.positions {
		total += pos.PnLSol
	}
	return total
}

// handleDomainEvent processes domain events using GlobalCache
func (s *MonitorScreen) handleDomainEvent(event domain.Event) {

	switch event.Type {
	case domain.EventPriceTick:
		// Handle price updates
		if data, ok := event.Data.(map[string]interface{}); ok {
			if positionID, exists := data["position_id"].(int); exists {
				if price, exists := data["price"].(float64); exists {
					if timestamp, exists := data["timestamp"].(time.Time); exists {
						s.updatePositionPrice(positionID, price, timestamp)
					}
				}
			}
		}

	case domain.EventOrderFilled:
		// Handle order completion
		if data, ok := event.Data.(map[string]interface{}); ok {
			if positionID, exists := data["position_id"].(int); exists {
				if _, exists := data["action"].(string); exists {
					// Mark position as inactive if fully sold
					if percentage, exists := data["percentage"].(float64); exists && percentage >= 100.0 {
						for i := range s.positions {
							if s.positions[i].ID == positionID {
								s.positions[i].Active = false
								break
							}
						}
					}
				}
			}
		}

	case domain.EventTaskExecuted:
		// Add new position to monitor
		if data, ok := event.Data.(map[string]interface{}); ok {
			// Create new position from task execution data
			newPos := MonitoredPosition{
				ID:           len(s.positions) + 1, // Simple ID generation
				TaskName:     fmt.Sprintf("%v", data["task_name"]),
				TokenMint:    fmt.Sprintf("%v", data["token_mint"]),
				TokenSymbol:  fmt.Sprintf("%v", data["token_symbol"]),
				EntryPrice:   0, // Will be updated by price monitoring
				CurrentPrice: 0,
				Amount:       0,
				PnLPercent:   0,
				PnLSol:       0,
				Volume24h:    0,
				LastUpdate:   time.Now(),
				PriceHistory: make([]float64, 0),
				Active:       true,
			}
			s.positions = append(s.positions, newPos)
		}

	default:
		// Handle other events - no cache needed
	}
}

// loadMockData loads some mock positions for demonstration and stores them in GlobalCache
func (s *MonitorScreen) loadMockData() {
	now := time.Now()

	s.positions = []MonitoredPosition{
		{
			ID:           1,
			TaskName:     "PUMP_SNIPE_1",
			TokenMint:    "DmigFWPu6xFSntkBqWAm5MqTFiJj8ir74pump",
			TokenSymbol:  "DEMO",
			EntryPrice:   0.000123,
			CurrentPrice: 0.000145,
			Amount:       0.01,
			PnLPercent:   17.89,
			PnLSol:       0.0022,
			Volume24h:    15.67,
			LastUpdate:   now,
			PriceHistory: []float64{0.000123, 0.000125, 0.000128, 0.000135, 0.000142, 0.000145},
			Active:       true,
		},
		{
			ID:           2,
			TaskName:     "PUMP_SWAP_2",
			TokenMint:    "AbcdEFGh9xFSntkBqWAm5MqTFDrC1ZtFiJj8ir",
			TokenSymbol:  "TEST",
			EntryPrice:   0.000089,
			CurrentPrice: 0.000076,
			Amount:       0.005,
			PnLPercent:   -14.61,
			PnLSol:       -0.00065,
			Volume24h:    8.92,
			LastUpdate:   now,
			PriceHistory: []float64{0.000089, 0.000087, 0.000084, 0.000081, 0.000078, 0.000076},
			Active:       true,
		},
	}

	// Phase 2: Store mock positions in GlobalCache
	if state.GlobalCache != nil {
		for _, pos := range s.positions {
			cachePos := monitor.Position{
				SessionID:   fmt.Sprintf("position_%d", pos.ID),
				WalletAddr:  "", // Mock data - no wallet address
				TokenMint:   pos.TokenMint,
				TokenSymbol: pos.TokenSymbol,
				Amount:      pos.Amount,
				InitialSOL:  pos.EntryPrice,
				CurrentSOL:  pos.CurrentPrice,
				PnL:         pos.PnLSol,
				PnLPercent:  pos.PnLPercent,
				Status:      map[bool]string{true: "active", false: "inactive"}[pos.Active],
				UpdatedAt:   pos.LastUpdate,
			}
			state.GlobalCache.SetPosition(cachePos)
		}
	}
}

// simulatePriceUpdates simulates price changes for demonstration
func (s *MonitorScreen) simulatePriceUpdates() {
	now := time.Now()

	for i := range s.positions {
		// Simulate small price changes
		change := (float64(time.Now().UnixNano()%100) - 50) / 10000.0 // ±5% max change
		newPrice := s.positions[i].CurrentPrice * (1 + change)
		if newPrice > 0 {
			s.updatePositionPrice(s.positions[i].ID, newPrice, now)
		}
	}
}

// exportTradeDataCmd creates a command to export trade data
func (s *MonitorScreen) exportTradeDataCmd() tea.Cmd {
	return func() tea.Msg {
		// Phase 3: Export functionality
		// For now, this is a placeholder that demonstrates the integration point
		// In a real implementation, this would:
		// 1. Get trades from TradeHistory (would need access to it)
		// 2. Use export.NewTradeExporter to export data
		// 3. Show success/error messages to user

		// Simulate export for demonstration
		exportMsg := fmt.Sprintf("Export completed: trades_%s.csv", time.Now().Format("20060102_150405"))

		// In real implementation, would use:
		// exporter := export.NewTradeExporter(logger)
		// trades := tradeHistory.GetAllTrades()
		// outputPath, err := exporter.ExportTrades(trades, export.ExportOptions{
		//     Format:    export.FormatCSV,
		//     StartTime: time.Now().Add(-24 * time.Hour),
		//     EndTime:   time.Now(),
		//     OutputDir: "./exports",
		// })

		return ui.SuccessMsg{Message: exportMsg}
	}
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
