package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines keyboard shortcuts for the application
type KeyMap struct {
	// Global navigation
	Quit key.Binding
	Back key.Binding
	Help key.Binding

	// Navigation
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Enter    key.Binding
	Tab      key.Binding
	ShiftTab key.Binding

	// Application specific
	CreateTask key.Binding
	RunTasks   key.Binding
	Monitor    key.Binding
	Settings   key.Binding
	Logs       key.Binding
	ToggleLogs key.Binding

	// Task management
	SelectTask key.Binding
	DeleteTask key.Binding
	EditTask   key.Binding
	SaveTask   key.Binding

	// Monitoring
	Sell     key.Binding
	Chart    key.Binding
	Refresh  key.Binding
	AutoSell key.Binding

	// Logs
	FilterInfo  key.Binding
	FilterWarn  key.Binding
	FilterError key.Binding
	ClearLogs   key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		// Global navigation
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?", "h"),
			key.WithHelp("?/h", "help"),
		),

		// Navigation
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev"),
		),

		// Application specific
		CreateTask: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "create task"),
		),
		RunTasks: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "run tasks"),
		),
		Monitor: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "monitor"),
		),
		Settings: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "settings"),
		),
		Logs: key.NewBinding(
			key.WithKeys("f12"),
			key.WithHelp("F12", "logs"),
		),
		ToggleLogs: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "toggle logs"),
		),

		// Task management
		SelectTask: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "select"),
		),
		DeleteTask: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		EditTask: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		SaveTask: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "save"),
		),

		// Monitoring
		Sell: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sell"),
		),
		Chart: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "chart"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r", "f5"),
			key.WithHelp("r/F5", "refresh"),
		),
		AutoSell: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "auto-sell"),
		),

		// Logs
		FilterInfo: key.NewBinding(
			key.WithKeys("f1"),
			key.WithHelp("F1", "info"),
		),
		FilterWarn: key.NewBinding(
			key.WithKeys("f2"),
			key.WithHelp("F2", "warn"),
		),
		FilterError: key.NewBinding(
			key.WithKeys("f3"),
			key.WithHelp("F3", "error"),
		),
		ClearLogs: key.NewBinding(
			key.WithKeys("ctrl+shift+l"),
			key.WithHelp("ctrl+shift+l", "clear logs"),
		),
	}
}

// ShortHelp returns key help text for the current context
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

// FullHelp returns extended help text for the current context
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.Tab, k.ShiftTab},
		{k.CreateTask, k.RunTasks, k.Monitor},
		{k.Settings, k.Logs, k.Help, k.Quit},
	}
}

// ContextualHelp returns help text based on the current route
func (k KeyMap) ContextualHelp(route Route) []key.Binding {
	switch route {
	case RouteMainMenu:
		return []key.Binding{k.Up, k.Down, k.Enter, k.Quit}
	case RouteCreateTask:
		return []key.Binding{k.Tab, k.ShiftTab, k.SaveTask, k.Back, k.Quit}
	case RouteTaskList:
		return []key.Binding{k.Up, k.Down, k.SelectTask, k.DeleteTask, k.EditTask, k.Back, k.Quit}
	case RouteMonitor:
		return []key.Binding{k.Sell, k.Chart, k.Refresh, k.AutoSell, k.Back, k.Quit}
	case RouteSettings:
		return []key.Binding{k.Tab, k.ShiftTab, k.SaveTask, k.Back, k.Quit}
	case RouteLogs:
		return []key.Binding{k.FilterInfo, k.FilterWarn, k.FilterError, k.ClearLogs, k.Back, k.Quit}
	default:
		return k.ShortHelp()
	}
}
