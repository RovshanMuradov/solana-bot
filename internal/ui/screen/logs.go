package screen

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/component"
	"github.com/rovshanmuradov/solana-bot/internal/ui/router"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// LogLevel represents different log levels
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
	LogLevelAll   LogLevel = "ALL"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Component string
	Message   string
	Raw       string
}

// LogsScreen represents the logs viewing screen
type LogsScreen struct {
	width  int
	height int
	keyMap ui.KeyMap

	// UI components
	helpBar    *component.HelpBar
	table      *component.Table
	filterForm *component.Form

	// State
	logs            []LogEntry
	filteredLogs    []LogEntry
	currentFilter   LogLevel
	searchTerm      string
	componentFilter string
	autoRefresh     bool
	refreshInterval time.Duration
	lastUpdate      time.Time
	scrollPosition  int
	errors          []string

	// Styling
	titleStyle     lipgloss.Style
	headerStyle    lipgloss.Style
	statusStyle    lipgloss.Style
	errorStyle     lipgloss.Style
	debugStyle     lipgloss.Style
	infoStyle      lipgloss.Style
	warnStyle      lipgloss.Style
	fatalStyle     lipgloss.Style
	timestampStyle lipgloss.Style
	componentStyle lipgloss.Style
	containerStyle lipgloss.Style

	// Configuration
	maxLogEntries  int
	showTimestamps bool
	showComponents bool
	showFilters    bool
	tailMode       bool // Follow new logs
}

// NewLogsScreen creates a new logs screen
func NewLogsScreen() *LogsScreen {
	palette := style.DefaultPalette()
	keyMap := ui.DefaultKeyMap()

	screen := &LogsScreen{
		keyMap:          keyMap,
		currentFilter:   LogLevelAll,
		autoRefresh:     true,
		refreshInterval: time.Second * 5,
		lastUpdate:      time.Now(),
		errors:          make([]string, 0),
		maxLogEntries:   1000,
		showTimestamps:  true,
		showComponents:  true,
		showFilters:     false,
		tailMode:        true,

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
			Bold(true),

		debugStyle: lipgloss.NewStyle().
			Foreground(palette.TextMuted),

		infoStyle: lipgloss.NewStyle().
			Foreground(palette.Text),

		warnStyle: lipgloss.NewStyle().
			Foreground(palette.Warning).
			Bold(true),

		fatalStyle: lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true).
			Background(palette.BackgroundAlt),

		timestampStyle: lipgloss.NewStyle().
			Foreground(palette.TextMuted),

		componentStyle: lipgloss.NewStyle().
			Foreground(palette.Secondary).
			Bold(true),

		containerStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Primary).
			Padding(1, 2).
			Margin(1, 0),
	}

	screen.initializeTable()
	screen.initializeFilterForm()
	screen.initializeHelpBar()

	return screen
}

// initializeTable sets up the logs table
func (s *LogsScreen) initializeTable() {
	s.table = component.NewTable().
		AddColumn("Time", 12, lipgloss.Left).
		AddColumn("Level", 8, lipgloss.Center).
		AddColumn("Component", 15, lipgloss.Left).
		AddColumn("Message", 50, lipgloss.Left).
		SetShowBorder(true).
		SetSelectable(true).
		SetZebra(false) // Custom coloring by log level
}

// initializeFilterForm sets up the filter form
func (s *LogsScreen) initializeFilterForm() {
	s.filterForm = component.NewForm().
		SetTitle("Log Filters").
		AddField("level", component.FieldTypeSelect, "Log Level", false, "Filter by log level").
		SetSelectOptions("level", []string{"ALL", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"}).
		AddField("search", component.FieldTypeText, "Search", false, "Search in log messages").
		AddField("component", component.FieldTypeText, "Component", false, "Filter by component name")

	// Set default values
	s.filterForm.SetFieldValue("level", "ALL")
}

// initializeHelpBar sets up the help bar
func (s *LogsScreen) initializeHelpBar() {
	s.helpBar = component.NewHelpBar().
		SetKeyBindings(s.keyMap.ContextualHelp(ui.RouteLogs)).
		SetCompact(false)
}

// Init initializes the logs screen
func (s *LogsScreen) Init() tea.Cmd {
	return tea.Batch(
		ui.ListenBus(),
		s.loadLogsCmd(),
		s.startAutoRefresh(),
	)
}

// LogsLoadedMsg is sent when logs are loaded
type LogsLoadedMsg struct {
	Logs  []LogEntry
	Error error
}

// RefreshLogsMsg is sent to trigger a refresh
type RefreshLogsMsg struct {
	Timestamp time.Time
}

// Update handles screen updates
func (s *LogsScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.showFilters {
			// Handle filter form input
			switch {
			case key.Matches(msg, s.keyMap.Back), msg.String() == "esc":
				s.showFilters = false
				s.applyFilters()

			case key.Matches(msg, s.keyMap.Enter):
				s.showFilters = false
				s.applyFilters()

			default:
				updatedForm, cmd := s.filterForm.Update(msg)
				s.filterForm = updatedForm
				cmds = append(cmds, cmd)
			}
		} else {
			// Handle main screen input
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
				s.tailMode = false // Disable tail mode when manually scrolling

			case key.Matches(msg, s.keyMap.Down):
				s.table.MoveDown()

			case key.Matches(msg, s.keyMap.Refresh):
				// Manual refresh
				cmds = append(cmds, s.loadLogsCmd())

			case msg.String() == "f":
				// Show filters
				s.showFilters = true

			case msg.String() == "c":
				// Clear logs
				s.logs = make([]LogEntry, 0)
				s.filteredLogs = make([]LogEntry, 0)
				s.updateTableDisplay()

			case msg.String() == "t":
				// Toggle tail mode
				s.tailMode = !s.tailMode
				if s.tailMode {
					s.scrollToBottom()
				}

			case msg.String() == "a":
				// Toggle auto-refresh
				s.autoRefresh = !s.autoRefresh
				if s.autoRefresh {
					cmds = append(cmds, s.startAutoRefresh())
				}

			case msg.String() == "s":
				// Toggle timestamps
				s.showTimestamps = !s.showTimestamps
				s.updateTableDisplay()

			case msg.String() == "m":
				// Toggle components
				s.showComponents = !s.showComponents
				s.updateTableDisplay()

			case msg.String() == "1":
				// Filter by ERROR
				s.currentFilter = LogLevelError
				s.filterForm.SetFieldValue("level", "ERROR")
				s.applyFilters()

			case msg.String() == "2":
				// Filter by WARN
				s.currentFilter = LogLevelWarn
				s.filterForm.SetFieldValue("level", "WARN")
				s.applyFilters()

			case msg.String() == "3":
				// Filter by INFO
				s.currentFilter = LogLevelInfo
				s.filterForm.SetFieldValue("level", "INFO")
				s.applyFilters()

			case msg.String() == "4":
				// Show all
				s.currentFilter = LogLevelAll
				s.filterForm.SetFieldValue("level", "ALL")
				s.applyFilters()
			}
		}

	case RefreshLogsMsg:
		s.lastUpdate = msg.Timestamp
		cmds = append(cmds, s.loadLogsCmd())
		if s.autoRefresh {
			cmds = append(cmds, s.startAutoRefresh())
		}

	case LogsLoadedMsg:
		if msg.Error != nil {
			s.errors = append(s.errors, fmt.Sprintf("Error loading logs: %v", msg.Error))
		} else {
			s.logs = msg.Logs
			s.applyFilters()
			if s.tailMode {
				s.scrollToBottom()
			}
		}

	case ui.DomainEventMsg:
		// Handle domain events (new log entries)

	case ui.ErrorMsg:
		s.errors = append(s.errors, msg.Error.Error())

	case ui.SuccessMsg:
		s.errors = make([]string, 0) // Clear errors on success
	}

	// Continue listening for events
	cmds = append(cmds, ui.ListenBus())

	return s, tea.Batch(cmds...)
}

// View renders the logs screen
func (s *LogsScreen) View() string {
	if s.width == 0 || s.height == 0 {
		return "Loading..."
	}

	var content strings.Builder

	// Title
	title := "📜 Application Logs"
	if s.autoRefresh {
		title += " (Auto-refresh ON)"
	}
	if s.tailMode {
		title += " (Tail mode)"
	}
	content.WriteString(s.titleStyle.Width(s.width).Render(title))
	content.WriteString("\n\n")

	// Status bar
	statusBar := s.renderStatusBar()
	content.WriteString(statusBar)
	content.WriteString("\n\n")

	// Error messages
	if len(s.errors) > 0 {
		for _, err := range s.errors[:min(len(s.errors), 2)] {
			content.WriteString(s.errorStyle.Render("❌ " + err))
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	// Filter form or logs table
	if s.showFilters {
		content.WriteString(s.containerStyle.Render(s.filterForm.View()))
	} else {
		// Logs table
		if len(s.filteredLogs) > 0 {
			content.WriteString(s.table.View())
		} else {
			emptyMsg := "No log entries match the current filters.\nPress 'f' to adjust filters or 'c' to clear."
			content.WriteString(s.statusStyle.Render(emptyMsg))
		}
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
func (s *LogsScreen) SetSize(width, height int) {
	s.width = width
	s.height = height
	s.helpBar.SetWidth(width)
	s.filterForm.SetWidth(width - 8)
	s.table.SetSize(width-4, height-15)
}

// renderStatusBar renders the status information
func (s *LogsScreen) renderStatusBar() string {
	var statusParts []string

	// Log count
	statusParts = append(statusParts, fmt.Sprintf("Total: %d", len(s.logs)))
	statusParts = append(statusParts, fmt.Sprintf("Filtered: %d", len(s.filteredLogs)))

	// Current filter
	if s.currentFilter != LogLevelAll {
		statusParts = append(statusParts, fmt.Sprintf("Filter: %s", s.currentFilter))
	}

	// Search term
	if s.searchTerm != "" {
		statusParts = append(statusParts, fmt.Sprintf("Search: '%s'", s.searchTerm))
	}

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

// renderInstructions renders usage instructions
func (s *LogsScreen) renderInstructions() string {
	var instructions []string

	if s.showFilters {
		instructions = append(instructions, "Enter/Esc: Apply filters")
	} else {
		instructions = append(instructions, "F: Filters")
		instructions = append(instructions, "1-4: Quick filter (Error/Warn/Info/All)")
		instructions = append(instructions, "T: Tail mode")
		instructions = append(instructions, "A: Auto-refresh")
		instructions = append(instructions, "S: Toggle timestamps")
		instructions = append(instructions, "M: Toggle components")
		instructions = append(instructions, "C: Clear logs")
		instructions = append(instructions, "F5: Refresh")
	}

	return s.statusStyle.Render(strings.Join(instructions, " • "))
}

// updateTableDisplay updates the table with current log data
func (s *LogsScreen) updateTableDisplay() {
	if s.table == nil {
		return
	}

	var rows [][]string

	for _, logEntry := range s.filteredLogs {
		var row []string

		// Timestamp
		timeStr := ""
		if s.showTimestamps {
			timeStr = logEntry.Timestamp.Format("15:04:05")
		}

		// Level with styling
		levelStr := string(logEntry.Level)

		// Component
		componentStr := ""
		if s.showComponents {
			componentStr = logEntry.Component
		}

		// Message (truncate if too long)
		messageStr := logEntry.Message
		if len(messageStr) > 80 {
			messageStr = messageStr[:77] + "..."
		}

		row = []string{timeStr, levelStr, componentStr, messageStr}
		rows = append(rows, row)
	}

	s.table.SetRows(rows)

	// Apply custom styling based on log levels
	for i, logEntry := range s.filteredLogs {
		style := s.getLogLevelStyle(logEntry.Level)
		s.table.SetRowStyle(i, style)
	}
}

// getLogLevelStyle returns the appropriate style for a log level
func (s *LogsScreen) getLogLevelStyle(level LogLevel) lipgloss.Style {
	switch level {
	case LogLevelDebug:
		return s.debugStyle
	case LogLevelInfo:
		return s.infoStyle
	case LogLevelWarn:
		return s.warnStyle
	case LogLevelError:
		return s.errorStyle
	case LogLevelFatal:
		return s.fatalStyle
	default:
		return s.infoStyle
	}
}

// applyFilters applies current filters to the logs
func (s *LogsScreen) applyFilters() {
	// Get filter values from form
	s.currentFilter = LogLevel(s.filterForm.GetFieldValue("level"))
	s.searchTerm = s.filterForm.GetFieldValue("search")
	s.componentFilter = s.filterForm.GetFieldValue("component")

	var filtered []LogEntry

	for _, logEntry := range s.logs {
		// Level filter
		if s.currentFilter != LogLevelAll && logEntry.Level != s.currentFilter {
			continue
		}

		// Search term filter
		if s.searchTerm != "" && !strings.Contains(strings.ToLower(logEntry.Message), strings.ToLower(s.searchTerm)) {
			continue
		}

		// Component filter
		if s.componentFilter != "" && !strings.Contains(strings.ToLower(logEntry.Component), strings.ToLower(s.componentFilter)) {
			continue
		}

		filtered = append(filtered, logEntry)
	}

	s.filteredLogs = filtered
	s.updateTableDisplay()
}

// scrollToBottom scrolls to the bottom of the log list
func (s *LogsScreen) scrollToBottom() {
	if len(s.filteredLogs) > 0 {
		s.table.SetSelectedRow(len(s.filteredLogs) - 1)
	}
}

// startAutoRefresh starts the auto-refresh timer
func (s *LogsScreen) startAutoRefresh() tea.Cmd {
	if !s.autoRefresh {
		return nil
	}

	return tea.Tick(s.refreshInterval, func(t time.Time) tea.Msg {
		return RefreshLogsMsg{Timestamp: t}
	})
}

// loadLogsCmd creates a command to load logs
func (s *LogsScreen) loadLogsCmd() tea.Cmd {
	return func() tea.Msg {
		logs, err := s.loadLogsFromFile()
		return LogsLoadedMsg{
			Logs:  logs,
			Error: err,
		}
	}
}

// loadLogsFromFile loads logs from the application log file
func (s *LogsScreen) loadLogsFromFile() ([]LogEntry, error) {
	// Try to find log files in common locations
	logPaths := []string{
		"logs/bot.log",
		"solana-bot.log",
		"/tmp/solana-bot.log",
		"app.log",
	}

	var logs []LogEntry

	for _, logPath := range logPaths {
		if _, err := os.Stat(logPath); err == nil {
			// File exists, try to read it
			fileEntries, err := s.parseLogFile(logPath)
			if err == nil {
				logs = append(logs, fileEntries...)
				break // Use the first found log file
			}
		}
	}

	// If no log file found, create some mock entries
	if len(logs) == 0 {
		logs = s.createMockLogs()
	}

	// Keep only the last maxLogEntries
	if len(logs) > s.maxLogEntries {
		logs = logs[len(logs)-s.maxLogEntries:]
	}

	return logs, nil
}

// parseLogFile parses a log file and returns log entries
func (s *LogsScreen) parseLogFile(filename string) ([]LogEntry, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var logs []LogEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if entry := s.parseLogLine(line); entry != nil {
			logs = append(logs, *entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return logs, err
	}

	return logs, nil
}

// parseLogLine parses a single log line
func (s *LogsScreen) parseLogLine(line string) *LogEntry {
	if line == "" {
		return nil
	}

	// Simple log format parsing
	// Expected format: YYYY-MM-DD HH:MM:SS [LEVEL] [COMPONENT] Message
	parts := strings.Fields(line)
	if len(parts) < 4 {
		// Fallback for simple lines
		return &LogEntry{
			Timestamp: time.Now(),
			Level:     LogLevelInfo,
			Component: "unknown",
			Message:   line,
			Raw:       line,
		}
	}

	// Try to parse timestamp
	var timestamp time.Time
	if len(parts) >= 2 {
		timeStr := parts[0] + " " + parts[1]
		if parsed, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
			timestamp = parsed
		} else {
			timestamp = time.Now()
		}
	}

	// Parse level (look for [LEVEL] pattern)
	level := LogLevelInfo
	component := "app"
	messageStart := 2

	for i, part := range parts[2:] {
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			content := strings.Trim(part, "[]")
			if s.isLogLevel(content) {
				level = LogLevel(content)
				messageStart = i + 3
			} else {
				component = content
				messageStart = i + 3
			}
		}
	}

	// Join remaining parts as message
	message := ""
	if messageStart < len(parts) {
		message = strings.Join(parts[messageStart:], " ")
	}

	return &LogEntry{
		Timestamp: timestamp,
		Level:     level,
		Component: component,
		Message:   message,
		Raw:       line,
	}
}

// isLogLevel checks if a string is a valid log level
func (s *LogsScreen) isLogLevel(str string) bool {
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}
	for _, level := range levels {
		if str == level {
			return true
		}
	}
	return false
}

// createMockLogs creates some mock log entries for demonstration
func (s *LogsScreen) createMockLogs() []LogEntry {
	now := time.Now()
	return []LogEntry{
		{
			Timestamp: now.Add(-time.Hour),
			Level:     LogLevelInfo,
			Component: "main",
			Message:   "Solana Trading Bot started",
			Raw:       fmt.Sprintf("%s [INFO] [main] Solana Trading Bot started", now.Add(-time.Hour).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-50 * time.Minute),
			Level:     LogLevelInfo,
			Component: "config",
			Message:   "Configuration loaded from configs/config.json",
			Raw:       fmt.Sprintf("%s [INFO] [config] Configuration loaded from configs/config.json", now.Add(-50*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-45 * time.Minute),
			Level:     LogLevelInfo,
			Component: "wallet",
			Message:   "Loaded 3 wallets from configs/wallets.csv",
			Raw:       fmt.Sprintf("%s [INFO] [wallet] Loaded 3 wallets from configs/wallets.csv", now.Add(-45*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-40 * time.Minute),
			Level:     LogLevelWarn,
			Component: "rpc",
			Message:   "RPC connection slow, retrying...",
			Raw:       fmt.Sprintf("%s [WARN] [rpc] RPC connection slow, retrying...", now.Add(-40*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-35 * time.Minute),
			Level:     LogLevelInfo,
			Component: "task",
			Message:   "Loaded 2 tasks from configs/tasks.csv",
			Raw:       fmt.Sprintf("%s [INFO] [task] Loaded 2 tasks from configs/tasks.csv", now.Add(-35*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-30 * time.Minute),
			Level:     LogLevelInfo,
			Component: "worker",
			Message:   "Started worker pool with 5 workers",
			Raw:       fmt.Sprintf("%s [INFO] [worker] Started worker pool with 5 workers", now.Add(-30*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-25 * time.Minute),
			Level:     LogLevelInfo,
			Component: "dex",
			Message:   "PumpFun adapter initialized",
			Raw:       fmt.Sprintf("%s [INFO] [dex] PumpFun adapter initialized", now.Add(-25*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-20 * time.Minute),
			Level:     LogLevelError,
			Component: "trade",
			Message:   "Failed to execute swap: insufficient balance",
			Raw:       fmt.Sprintf("%s [ERROR] [trade] Failed to execute swap: insufficient balance", now.Add(-20*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-15 * time.Minute),
			Level:     LogLevelInfo,
			Component: "monitor",
			Message:   "Started price monitoring for DEMO token",
			Raw:       fmt.Sprintf("%s [INFO] [monitor] Started price monitoring for DEMO token", now.Add(-15*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-10 * time.Minute),
			Level:     LogLevelWarn,
			Component: "monitor",
			Message:   "Price drop detected: -15% in 5 minutes",
			Raw:       fmt.Sprintf("%s [WARN] [monitor] Price drop detected: -15%% in 5 minutes", now.Add(-10*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-5 * time.Minute),
			Level:     LogLevelInfo,
			Component: "trade",
			Message:   "Auto-sell triggered for 50% of position",
			Raw:       fmt.Sprintf("%s [INFO] [trade] Auto-sell triggered for 50%% of position", now.Add(-5*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-2 * time.Minute),
			Level:     LogLevelDebug,
			Component: "rpc",
			Message:   "Sending transaction: 3mZ8...k9pQ",
			Raw:       fmt.Sprintf("%s [DEBUG] [rpc] Sending transaction: 3mZ8...k9pQ", now.Add(-2*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now.Add(-1 * time.Minute),
			Level:     LogLevelInfo,
			Component: "trade",
			Message:   "Transaction confirmed successfully",
			Raw:       fmt.Sprintf("%s [INFO] [trade] Transaction confirmed successfully", now.Add(-1*time.Minute).Format("2006-01-02 15:04:05")),
		},
		{
			Timestamp: now,
			Level:     LogLevelInfo,
			Component: "monitor",
			Message:   "Real-time monitoring active for 1 positions",
			Raw:       fmt.Sprintf("%s [INFO] [monitor] Real-time monitoring active for 1 positions", now.Format("2006-01-02 15:04:05")),
		},
	}
}

// GetLogCount returns the number of logs
func (s *LogsScreen) GetLogCount() int {
	return len(s.logs)
}

// GetFilteredLogCount returns the number of filtered logs
func (s *LogsScreen) GetFilteredLogCount() int {
	return len(s.filteredLogs)
}

// GetCurrentFilter returns the current log level filter
func (s *LogsScreen) GetCurrentFilter() LogLevel {
	return s.currentFilter
}
