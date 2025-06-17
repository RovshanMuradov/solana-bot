package style

import "github.com/charmbracelet/lipgloss"

// Color palette based on o3's modern design recommendations
var (
	// Primary colors
	Cyan    = lipgloss.Color("#00E5FF") // Primary highlight
	Magenta = lipgloss.Color("#FF1B6B") // Accent / buttons
	Yellow  = lipgloss.Color("#FFB500") // Warnings
	Green   = lipgloss.Color("#2AFFAA") // Positive PnL / success
	Red     = lipgloss.Color("#FF5555") // Negative PnL / errors
	Blue    = lipgloss.Color("#3B82F6") // Info / links
	Purple  = lipgloss.Color("#8B5CF6") // Secondary accent

	// Base colors
	Base03 = lipgloss.Color("#1B1D23") // Background
	Base02 = lipgloss.Color("#262831") // Darker background
	Base01 = lipgloss.Color("#6C7280") // Muted text
	Base2  = lipgloss.Color("#ECEFF4") // Primary text
	Base1  = lipgloss.Color("#B4BCC8") // Secondary text

	// Trading specific colors
	BuyColor  = Green
	SellColor = Red
	HoldColor = Yellow

	// Status colors
	SuccessColor = Green
	ErrorColor   = Red
	WarningColor = Yellow
	InfoColor    = Blue
)

// Palette provides a centralized color management
type Palette struct {
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Success   lipgloss.Color
	Error     lipgloss.Color
	Warning   lipgloss.Color
	Info      lipgloss.Color

	Background    lipgloss.Color
	BackgroundAlt lipgloss.Color
	Text          lipgloss.Color
	TextMuted     lipgloss.Color
	TextSecondary lipgloss.Color

	Buy  lipgloss.Color
	Sell lipgloss.Color
	Hold lipgloss.Color
}

// DefaultPalette returns the default color palette
func DefaultPalette() Palette {
	return Palette{
		Primary:   Cyan,
		Secondary: Magenta,
		Success:   Green,
		Error:     Red,
		Warning:   Yellow,
		Info:      Blue,

		Background:    Base03,
		BackgroundAlt: Base02,
		Text:          Base2,
		TextMuted:     Base01,
		TextSecondary: Base1,

		Buy:  BuyColor,
		Sell: SellColor,
		Hold: HoldColor,
	}
}
