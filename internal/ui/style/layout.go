package style

import (
	"github.com/charmbracelet/lipgloss"
)

var palette = DefaultPalette()

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Background(palette.Background).
			Foreground(palette.Primary).
			Bold(true).
			Padding(0, 2).
			Margin(0, 0, 1, 0)

	SubHeaderStyle = lipgloss.NewStyle().
			Foreground(palette.Secondary).
			Bold(true).
			Margin(0, 0, 1, 0)

	TitleStyle = lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true).
			Margin(1, 0)
)

// Layout styles
var (
	ContainerStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Margin(0, 1)

	SidebarStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Primary).
			Width(30)

	MainContentStyle = lipgloss.NewStyle().
				Padding(1, 2).
				Margin(0, 1)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.TextMuted).
			Padding(1, 2).
			Margin(0, 1)

	ActivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(palette.Primary).
				Padding(1, 2).
				Margin(0, 1)
)

// Table styles
var (
	TableHeaderStyle = lipgloss.NewStyle().
				Foreground(palette.Secondary).
				Bold(true).
				Padding(0, 1).
				Margin(0, 0, 1, 0)

	TableRowStyle = lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(0, 1)

	TableRowSelectedStyle = lipgloss.NewStyle().
				Foreground(palette.Background).
				Background(palette.Primary).
				Padding(0, 1)

	TableCellStyle = lipgloss.NewStyle().
			Padding(0, 1)
)

// Button styles
var (
	ButtonStyle = lipgloss.NewStyle().
			Foreground(palette.Background).
			Background(palette.Secondary).
			Padding(0, 2).
			Margin(0, 1).
			Bold(true)

	ButtonActiveStyle = lipgloss.NewStyle().
				Foreground(palette.Background).
				Background(palette.Primary).
				Padding(0, 2).
				Margin(0, 1).
				Bold(true)

	ButtonDisabledStyle = lipgloss.NewStyle().
				Foreground(palette.TextMuted).
				Background(palette.BackgroundAlt).
				Padding(0, 2).
				Margin(0, 1)
)

// Form styles
var (
	FormLabelStyle = lipgloss.NewStyle().
			Foreground(palette.Text).
			Bold(true).
			Margin(0, 1, 0, 0)

	FormInputStyle = lipgloss.NewStyle().
			Foreground(palette.Text).
			Background(palette.BackgroundAlt).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.TextMuted)

	FormInputFocusedStyle = lipgloss.NewStyle().
				Foreground(palette.Text).
				Background(palette.BackgroundAlt).
				Padding(0, 1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(palette.Primary)

	FormErrorStyle = lipgloss.NewStyle().
			Foreground(palette.Error).
			Margin(0, 1)
)

// Status styles
var (
	SuccessStyle = lipgloss.NewStyle().
			Foreground(palette.Success).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true)

	WarningStyle = lipgloss.NewStyle().
			Foreground(palette.Warning).
			Bold(true)

	InfoStyle = lipgloss.NewStyle().
			Foreground(palette.Info)
)

// Trading styles
var (
	BuyStyle = lipgloss.NewStyle().
			Foreground(palette.Buy).
			Bold(true)

	SellStyle = lipgloss.NewStyle().
			Foreground(palette.Sell).
			Bold(true)

	HoldStyle = lipgloss.NewStyle().
			Foreground(palette.Hold).
			Bold(true)

	ProfitStyle = lipgloss.NewStyle().
			Foreground(palette.Success).
			Bold(true)

	LossStyle = lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true)
)

// Progress bar styles
var (
	ProgressBarStyle = lipgloss.NewStyle().
				Foreground(palette.Primary).
				Background(palette.BackgroundAlt)

	ProgressBarFilledStyle = lipgloss.NewStyle().
				Background(palette.Primary)
)

// Help bar style
var (
	HelpStyle = lipgloss.NewStyle().
		Foreground(palette.TextMuted).
		Margin(1, 0, 0, 0).
		Italic(true)
)

// Adaptive layout helpers
func AdaptiveJoinHorizontal(width int, styles ...string) string {
	if width < 80 {
		// Stack vertically on narrow screens
		return lipgloss.JoinVertical(lipgloss.Left, styles...)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, styles...)
}

func AdaptiveWidth(width, percentage int) int {
	if width < 80 {
		return width - 4 // Leave some margin on narrow screens
	}
	return (width * percentage) / 100
}
