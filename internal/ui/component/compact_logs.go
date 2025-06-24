package component

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/logger"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// LogFilter defines what log levels to show
type LogFilter struct {
	ShowError   bool
	ShowWarning bool
	ShowInfo    bool
	ShowDebug   bool
}

// CompactLogViewer provides a compact log viewer integration
type CompactLogViewer struct {
	buffer   *logger.LogBuffer // Use existing LogBuffer
	viewport viewport.Model
	filter   LogFilter
	style    CompactLogStyle
	width    int
	height   int
	visible  bool
	title    string
}

// CompactLogStyle contains all styling for the log viewer
type CompactLogStyle struct {
	container lipgloss.Style
	title     lipgloss.Style
	entry     lipgloss.Style
	timestamp lipgloss.Style
	error     lipgloss.Style
	warning   lipgloss.Style
	info      lipgloss.Style
	debug     lipgloss.Style
}

// NewCompactLogViewer creates a new compact log viewer
func NewCompactLogViewer(logBuffer *logger.LogBuffer) *CompactLogViewer {
	palette := style.DefaultPalette()

	return &CompactLogViewer{
		buffer:  logBuffer,
		visible: true,
		title:   "Recent Logs",
		filter: LogFilter{
			ShowError:   true,
			ShowWarning: true,
			ShowInfo:    true,
			ShowDebug:   false, // Hide debug by default for compact view
		},
		style: CompactLogStyle{
			container: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(palette.Info).
				Padding(1, 2).
				MarginTop(1),

			title: lipgloss.NewStyle().
				Foreground(palette.Info).
				Bold(true),

			entry: lipgloss.NewStyle().
				Foreground(palette.Text).
				Padding(0, 1),

			timestamp: lipgloss.NewStyle().
				Foreground(palette.TextMuted),

			error: lipgloss.NewStyle().
				Foreground(palette.Error).
				Bold(true),

			warning: lipgloss.NewStyle().
				Foreground(palette.Warning).
				Bold(true),

			info: lipgloss.NewStyle().
				Foreground(palette.Info),

			debug: lipgloss.NewStyle().
				Foreground(palette.TextMuted),
		},
		viewport: viewport.New(50, 4), // Default size
	}
}

// SetSize sets the component dimensions
func (clv *CompactLogViewer) SetSize(width, height int) {
	clv.width = width
	clv.height = height
	clv.style.container = clv.style.container.Width(width - 4)

	// Viewport size accounts for borders and title
	viewportWidth := width - 6   // Border + padding
	viewportHeight := height - 4 // Border + title + padding

	if viewportHeight < 2 {
		viewportHeight = 2
	}

	clv.viewport.Width = viewportWidth
	clv.viewport.Height = viewportHeight
}

// SetVisible toggles the visibility of the log viewer
func (clv *CompactLogViewer) SetVisible(visible bool) {
	clv.visible = visible
}

// IsVisible returns whether the log viewer is visible
func (clv *CompactLogViewer) IsVisible() bool {
	return clv.visible
}

// SetFilter updates the log filter
func (clv *CompactLogViewer) SetFilter(filter LogFilter) {
	clv.filter = filter
	clv.updateViewport() // Refresh content with new filter
}

// ToggleLogLevel toggles a specific log level
func (clv *CompactLogViewer) ToggleLogLevel(level string) {
	switch level {
	case "error":
		clv.filter.ShowError = !clv.filter.ShowError
	case "warning":
		clv.filter.ShowWarning = !clv.filter.ShowWarning
	case "info":
		clv.filter.ShowInfo = !clv.filter.ShowInfo
	case "debug":
		clv.filter.ShowDebug = !clv.filter.ShowDebug
	}
	clv.updateViewport()
}

// Update handles viewport updates
func (clv *CompactLogViewer) Update(msg tea.Msg) tea.Cmd {
	if !clv.visible {
		return nil
	}

	var cmd tea.Cmd
	clv.viewport, cmd = clv.viewport.Update(msg)

	// Refresh content from log buffer
	clv.updateViewport()

	return cmd
}

// View renders the compact log viewer
func (clv *CompactLogViewer) View() string {
	if !clv.visible {
		return ""
	}

	// Update content before rendering
	clv.updateViewport()

	title := fmt.Sprintf("%s [L]Toggle", clv.title)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		clv.style.title.Render(title),
		clv.viewport.View(),
	)

	return clv.style.container.Render(content)
}

// updateViewport refreshes the viewport content from log buffer
func (clv *CompactLogViewer) updateViewport() {
	if clv.buffer == nil {
		clv.viewport.SetContent("No log buffer available")
		return
	}

	// Get recent entries from the existing LogBuffer
	entries := clv.buffer.GetRecentLogs(50) // Get last 50 entries

	var filteredEntries []string
	for _, entry := range entries {
		if clv.shouldShowEntry(entry) {
			formatted := clv.formatLogEntry(entry)
			filteredEntries = append(filteredEntries, formatted)
		}
	}

	// If no entries match filter, show message
	if len(filteredEntries) == 0 {
		clv.viewport.SetContent("No logs match current filter")
		return
	}

	// Join all entries and set viewport content
	content := strings.Join(filteredEntries, "\n")
	clv.viewport.SetContent(content)

	// Auto-scroll to bottom for new entries
	clv.viewport.GotoBottom()
}

// shouldShowEntry determines if a log entry should be displayed based on filter
func (clv *CompactLogViewer) shouldShowEntry(entry logger.LogEntry) bool {
	switch strings.ToLower(entry.Level) {
	case "error":
		return clv.filter.ShowError
	case "warning", "warn":
		return clv.filter.ShowWarning
	case "info":
		return clv.filter.ShowInfo
	case "debug":
		return clv.filter.ShowDebug
	default:
		return clv.filter.ShowInfo // Default to info level
	}
}

// formatLogEntry formats a log entry for display
func (clv *CompactLogViewer) formatLogEntry(entry logger.LogEntry) string {
	// Format timestamp
	timestamp := clv.style.timestamp.Render(entry.Timestamp.Format("15:04:05"))

	// Style message based on level
	var styledMessage string
	switch strings.ToLower(entry.Level) {
	case "error":
		styledMessage = clv.style.error.Render(entry.Message)
	case "warning", "warn":
		styledMessage = clv.style.warning.Render(entry.Message)
	case "info":
		styledMessage = clv.style.info.Render(entry.Message)
	case "debug":
		styledMessage = clv.style.debug.Render(entry.Message)
	default:
		styledMessage = clv.style.entry.Render(entry.Message)
	}

	// Combine timestamp and message
	return fmt.Sprintf("%s %s", timestamp, styledMessage)
}

// GetHeight returns the component height for layout calculations
func (clv *CompactLogViewer) GetHeight() int {
	if !clv.visible {
		return 0
	}
	return clv.height
}

// GetFilterStatus returns current filter status as string
func (clv *CompactLogViewer) GetFilterStatus() string {
	var active []string
	if clv.filter.ShowError {
		active = append(active, "Error")
	}
	if clv.filter.ShowWarning {
		active = append(active, "Warning")
	}
	if clv.filter.ShowInfo {
		active = append(active, "Info")
	}
	if clv.filter.ShowDebug {
		active = append(active, "Debug")
	}

	if len(active) == 0 {
		return "No filters active"
	}

	return fmt.Sprintf("Showing: %s", strings.Join(active, ", "))
}

// ScrollUp scrolls the log viewer up
func (clv *CompactLogViewer) ScrollUp() {
	clv.viewport.LineUp(1)
}

// ScrollDown scrolls the log viewer down
func (clv *CompactLogViewer) ScrollDown() {
	clv.viewport.LineDown(1)
}

// ScrollToTop scrolls to the top of logs
func (clv *CompactLogViewer) ScrollToTop() {
	clv.viewport.GotoTop()
}

// ScrollToBottom scrolls to the bottom of logs
func (clv *CompactLogViewer) ScrollToBottom() {
	clv.viewport.GotoBottom()
}
