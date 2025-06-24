package component

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/gaming"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
	"github.com/rovshanmuradov/solana-bot/internal/ui/types"
)

// FocusPane provides detailed view of a selected position with minimal emojis
type FocusPane struct {
	position    *types.MonitoredPosition
	sparkline   *Sparkline
	gamingLevel gaming.GamingLevel
	style       FocusPaneStyle
	width       int
	visible     bool
}

// FocusPaneStyle contains all styling for the focus pane
type FocusPaneStyle struct {
	container     lipgloss.Style
	title         lipgloss.Style
	pricePositive lipgloss.Style
	priceNegative lipgloss.Style
	priceNeutral  lipgloss.Style
	stats         lipgloss.Style
	hotkeys       lipgloss.Style
	level         lipgloss.Style
}

// NewFocusPane creates a new focus pane component
func NewFocusPane() *FocusPane {
	palette := style.DefaultPalette()

	return &FocusPane{
		visible: true,
		style: FocusPaneStyle{
			container: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(palette.Secondary).
				Padding(1, 2).
				MarginTop(1).
				MarginBottom(1),

			title: lipgloss.NewStyle().
				Foreground(palette.Primary).
				Bold(true).
				Align(lipgloss.Center),

			pricePositive: lipgloss.NewStyle().
				Foreground(palette.Success).
				Bold(true),

			priceNegative: lipgloss.NewStyle().
				Foreground(palette.Error).
				Bold(true),

			priceNeutral: lipgloss.NewStyle().
				Foreground(palette.TextMuted).
				Bold(false),

			stats: lipgloss.NewStyle().
				Foreground(palette.Text).
				Padding(0, 1),

			hotkeys: lipgloss.NewStyle().
				Foreground(palette.TextMuted).
				Italic(true).
				Align(lipgloss.Center),

			level: lipgloss.NewStyle().
				Foreground(palette.Secondary).
				Bold(true),
		},
		sparkline: NewSparkline(40), // Create sparkline for price chart
	}
}

// SetPosition updates the focused position
func (fp *FocusPane) SetPosition(pos *types.MonitoredPosition) {
	fp.position = pos
	if pos != nil {
		fp.gamingLevel = gaming.CalculateLevel(pos.PnLSol)
		// Update sparkline with price history
		if len(pos.PriceHistory) > 0 {
			fp.sparkline.SetData(pos.PriceHistory).SetWidth(40)
		}
	}
}

// SetVisible toggles the visibility of the focus pane
func (fp *FocusPane) SetVisible(visible bool) {
	fp.visible = visible
}

// IsVisible returns whether the focus pane is visible
func (fp *FocusPane) IsVisible() bool {
	return fp.visible
}

// SetWidth sets the component width for responsive layout
func (fp *FocusPane) SetWidth(width int) {
	fp.width = width
	fp.style.container = fp.style.container.Width(width - 4)
	if fp.sparkline != nil {
		chartWidth := width - 20 // Leave space for borders and padding
		if chartWidth > 40 {
			chartWidth = 40 // Max chart width
		}
		if chartWidth < 10 {
			chartWidth = 10 // Min chart width
		}
		fp.sparkline.SetWidth(chartWidth)
	}
}

// View renders the focus pane
func (fp *FocusPane) View() string {
	if !fp.visible || fp.position == nil {
		return ""
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		fp.renderTitle(),
		fp.renderPriceChart(),
		fp.renderStats(),
		fp.renderHotkeys(),
	)

	return fp.style.container.Render(content)
}

// renderTitle renders the title with token symbol and gaming level
func (fp *FocusPane) renderTitle() string {
	levelStyle := gaming.NewLevelBadgeStyle(fp.gamingLevel)
	levelBadge := levelStyle.RenderBadge(fp.gamingLevel)

	title := fmt.Sprintf("Focus: %s (%s %s)",
		fp.position.TokenSymbol,
		levelBadge,
		fp.gamingLevel.Title,
	)

	return fp.style.title.Render(title)
}

// renderPriceChart renders the price trend with sparkline
func (fp *FocusPane) renderPriceChart() string {
	// Price trend with minimal emoji (only for direction)
	var trendEmoji string
	var priceStyle lipgloss.Style

	if fp.position.PnLPercent > 0 {
		trendEmoji = "📈"
		priceStyle = fp.style.pricePositive
	} else if fp.position.PnLPercent < 0 {
		trendEmoji = "📉"
		priceStyle = fp.style.priceNegative
	} else {
		trendEmoji = ""
		priceStyle = fp.style.priceNeutral
	}

	// Price trend line
	var trendText string
	if trendEmoji != "" {
		trendText = fmt.Sprintf("Price Trend: %.2f%% %s", fp.position.PnLPercent, trendEmoji)
	} else {
		trendText = fmt.Sprintf("Price Trend: %.2f%%", fp.position.PnLPercent)
	}

	priceTrend := priceStyle.Render(trendText)

	// Sparkline chart
	chartView := ""
	if fp.sparkline != nil && len(fp.position.PriceHistory) > 1 {
		chartView = fp.sparkline.View()
	} else {
		chartView = "Insufficient data for chart"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		priceTrend,
		fp.style.stats.Render(chartView),
	)
}

// renderStats renders position statistics
func (fp *FocusPane) renderStats() string {
	leftColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		fmt.Sprintf("Entry: %.8f SOL", fp.position.EntryPrice),
		fmt.Sprintf("Current: %.8f SOL", fp.position.CurrentPrice),
	)

	rightColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		fmt.Sprintf("Invested: %.6f SOL", fp.position.Amount),
		fmt.Sprintf("Value: %.6f SOL", fp.position.Amount*fp.position.CurrentPrice/fp.position.EntryPrice),
	)

	stats := lipgloss.JoinHorizontal(
		lipgloss.Top,
		fp.style.stats.Render(leftColumn),
		fp.style.stats.Width(20).Render("│"), // Separator
		fp.style.stats.Render(rightColumn),
	)

	// Last update info
	lastUpdate := fmt.Sprintf("Last: %s", fp.position.LastUpdate.Format("15:04:05"))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		stats,
		fp.style.stats.Render(lastUpdate),
	)
}

// renderHotkeys renders hotkey instructions without emojis
func (fp *FocusPane) renderHotkeys() string {
	hotkeys := "[S]ell Menu  [T]P/SL  [M]ore  [1-5] Quick Sell"
	return fp.style.hotkeys.Render(hotkeys)
}

// GetHeight returns the component height for layout calculations
func (fp *FocusPane) GetHeight() int {
	if !fp.visible || fp.position == nil {
		return 0
	}
	return 8 // Border + padding + content lines
}

// Update updates the focus pane with new data (called by parent screen)
func (fp *FocusPane) Update() {
	// This method can be called by MonitorScreen to update data
	// Currently position updates are handled via SetPosition
	// Future: could integrate with GlobalCache here
}
