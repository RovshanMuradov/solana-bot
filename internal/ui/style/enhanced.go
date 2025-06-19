package style

import (
	"github.com/charmbracelet/lipgloss"
)

// Enhanced styles for new UI components

// HeaderStyles provides styling for the status header
type HeaderStyles struct {
	Container   lipgloss.Style
	Title       lipgloss.Style
	Wallet      lipgloss.Style
	RPCGood     lipgloss.Style
	RPCBad      lipgloss.Style
	PnLPositive lipgloss.Style
	PnLNegative lipgloss.Style
	PnLNeutral  lipgloss.Style
}

// NewHeaderStyles creates header styles with the given palette
func NewHeaderStyles(palette Palette) HeaderStyles {
	return HeaderStyles{
		Container: lipgloss.NewStyle().
			Background(palette.Background).
			Foreground(palette.Text).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Primary).
			Padding(0, 2).
			MarginBottom(1),

		Title: lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true),

		Wallet: lipgloss.NewStyle().
			Foreground(palette.TextSecondary).
			Bold(false),

		RPCGood: lipgloss.NewStyle().
			Foreground(palette.Success).
			Bold(true),

		RPCBad: lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true),

		PnLPositive: lipgloss.NewStyle().
			Foreground(palette.Success).
			Bold(true),

		PnLNegative: lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true),

		PnLNeutral: lipgloss.NewStyle().
			Foreground(palette.TextMuted).
			Bold(false),
	}
}

// FocusStyles provides styling for the focus pane
type FocusStyles struct {
	Container     lipgloss.Style
	Title         lipgloss.Style
	PricePositive lipgloss.Style
	PriceNegative lipgloss.Style
	PriceNeutral  lipgloss.Style
	Stats         lipgloss.Style
	Hotkeys       lipgloss.Style
	Level         lipgloss.Style
}

// NewFocusStyles creates focus pane styles
func NewFocusStyles(palette Palette) FocusStyles {
	return FocusStyles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Secondary).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1),

		Title: lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true).
			Align(lipgloss.Center),

		PricePositive: lipgloss.NewStyle().
			Foreground(palette.Success).
			Bold(true),

		PriceNegative: lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true),

		PriceNeutral: lipgloss.NewStyle().
			Foreground(palette.TextMuted).
			Bold(false),

		Stats: lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(0, 1),

		Hotkeys: lipgloss.NewStyle().
			Foreground(palette.TextMuted).
			Italic(true).
			Align(lipgloss.Center),

		Level: lipgloss.NewStyle().
			Foreground(palette.Secondary).
			Bold(true),
	}
}

// LogStyles provides styling for the compact log viewer
type LogStyles struct {
	Container lipgloss.Style
	Title     lipgloss.Style
	Entry     lipgloss.Style
	Timestamp lipgloss.Style
	Error     lipgloss.Style
	Warning   lipgloss.Style
	Info      lipgloss.Style
	Debug     lipgloss.Style
}

// NewLogStyles creates compact log viewer styles
func NewLogStyles(palette Palette) LogStyles {
	return LogStyles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Info).
			Padding(1, 2).
			MarginTop(1),

		Title: lipgloss.NewStyle().
			Foreground(palette.Info).
			Bold(true),

		Entry: lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(0, 1),

		Timestamp: lipgloss.NewStyle().
			Foreground(palette.TextMuted),

		Error: lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true),

		Warning: lipgloss.NewStyle().
			Foreground(palette.Warning).
			Bold(true),

		Info: lipgloss.NewStyle().
			Foreground(palette.Info),

		Debug: lipgloss.NewStyle().
			Foreground(palette.TextMuted),
	}
}

// GamingColors provides additional colors for gaming elements
type GamingColors struct {
	Rookie lipgloss.Color
	Trader lipgloss.Color
	Pro    lipgloss.Color
	Master lipgloss.Color
	Legend lipgloss.Color
}

// DefaultGamingColors returns the default gaming color palette
func DefaultGamingColors() GamingColors {
	return GamingColors{
		Rookie: lipgloss.Color("#6C7280"), // Base01 - muted
		Trader: lipgloss.Color("#FFB500"), // Yellow
		Pro:    lipgloss.Color("#3B82F6"), // Blue
		Master: lipgloss.Color("#8B5CF6"), // Purple
		Legend: lipgloss.Color("#2AFFAA"), // Green
	}
}

// MonitorEnhancedStyles combines all enhanced styles for monitor screen
type MonitorEnhancedStyles struct {
	Header HeaderStyles
	Focus  FocusStyles
	Logs   LogStyles
	Gaming GamingColors
}

// NewMonitorEnhancedStyles creates all enhanced styles for monitor
func NewMonitorEnhancedStyles() MonitorEnhancedStyles {
	palette := DefaultPalette()

	return MonitorEnhancedStyles{
		Header: NewHeaderStyles(palette),
		Focus:  NewFocusStyles(palette),
		Logs:   NewLogStyles(palette),
		Gaming: DefaultGamingColors(),
	}
}
