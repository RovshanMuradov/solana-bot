package component

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// PnLGauge represents a profit/loss gauge component
type PnLGauge struct {
	value       float64 // PnL percentage
	width       int
	showValue   bool
	showPercent bool
	style       lipgloss.Style

	// Thresholds for color coding
	profitThreshold float64
	lossThreshold   float64
}

// NewPnLGauge creates a new PnL gauge component
func NewPnLGauge(width int) *PnLGauge {
	return &PnLGauge{
		value:           0,
		width:           width,
		showValue:       true,
		showPercent:     true,
		style:           lipgloss.NewStyle(),
		profitThreshold: 5.0,  // 5% profit threshold
		lossThreshold:   -5.0, // 5% loss threshold
	}
}

// SetValue sets the PnL percentage value
func (p *PnLGauge) SetValue(value float64) *PnLGauge {
	p.value = value
	return p
}

// SetWidth sets the gauge width
func (p *PnLGauge) SetWidth(width int) *PnLGauge {
	p.width = width
	return p
}

// SetShowValue enables/disables value display
func (p *PnLGauge) SetShowValue(show bool) *PnLGauge {
	p.showValue = show
	return p
}

// SetShowPercent enables/disables percentage symbol
func (p *PnLGauge) SetShowPercent(show bool) *PnLGauge {
	p.showPercent = show
	return p
}

// SetThresholds sets the profit and loss thresholds for color coding
func (p *PnLGauge) SetThresholds(profitThreshold, lossThreshold float64) *PnLGauge {
	p.profitThreshold = profitThreshold
	p.lossThreshold = lossThreshold
	return p
}

// View renders the PnL gauge
func (p *PnLGauge) View() string {
	palette := style.DefaultPalette()

	// Determine color based on value
	var color lipgloss.Color
	var arrow string

	switch {
	case p.value >= p.profitThreshold:
		color = palette.Success
		arrow = "↗"
	case p.value <= p.lossThreshold:
		color = palette.Error
		arrow = "↘"
	case p.value > 0:
		color = palette.Success
		arrow = "↑"
	case p.value < 0:
		color = palette.Error
		arrow = "↓"
	default:
		color = palette.TextMuted
		arrow = "→"
	}

	// Generate the gauge bar
	gaugeBar := p.generateGaugeBar()

	// Style the gauge
	styledGauge := lipgloss.NewStyle().Foreground(color).Render(gaugeBar)

	// Add value text if enabled
	if p.showValue {
		valueText := fmt.Sprintf("%.2f", math.Abs(p.value))
		if p.showPercent {
			valueText += "%"
		}

		prefix := ""
		if p.value > 0 {
			prefix = "+"
		} else if p.value < 0 {
			prefix = "-"
		}

		fullText := prefix + valueText + " " + arrow
		styledText := lipgloss.NewStyle().Foreground(color).Bold(true).Render(fullText)

		return styledGauge + " " + styledText
	}

	return styledGauge
}

// generateGaugeBar creates the visual gauge representation
func (p *PnLGauge) generateGaugeBar() string {
	if p.width <= 0 {
		return ""
	}

	// Gauge characters for different intensities
	chars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

	// Calculate the intensity based on absolute value
	// Scale: 0-10% maps to different intensities
	absValue := math.Abs(p.value)
	maxScale := 20.0 // 20% is considered "full scale"

	// Normalize to 0-1 range
	intensity := math.Min(absValue/maxScale, 1.0)

	// Map to character index
	charIndex := int(intensity * float64(len(chars)-1))
	if charIndex >= len(chars) {
		charIndex = len(chars) - 1
	}

	// Calculate how many characters to show
	filledWidth := int(intensity * float64(p.width))
	if filledWidth < 1 && absValue > 0 {
		filledWidth = 1 // Show at least one character for non-zero values
	}

	var result strings.Builder

	// Fill the gauge
	for i := 0; i < p.width; i++ {
		if i < filledWidth {
			result.WriteString(chars[charIndex])
		} else {
			result.WriteString("▁")
		}
	}

	return result.String()
}

// GetStatus returns a text status based on the current value
func (p *PnLGauge) GetStatus() string {
	switch {
	case p.value >= p.profitThreshold:
		return "Strong Profit"
	case p.value > 0:
		return "Profit"
	case p.value <= p.lossThreshold:
		return "Strong Loss"
	case p.value < 0:
		return "Loss"
	default:
		return "Break Even"
	}
}

// GetColor returns the current color based on the value
func (p *PnLGauge) GetColor() lipgloss.Color {
	palette := style.DefaultPalette()

	switch {
	case p.value >= p.profitThreshold:
		return palette.Success
	case p.value <= p.lossThreshold:
		return palette.Error
	case p.value > 0:
		return palette.Success
	case p.value < 0:
		return palette.Error
	default:
		return palette.TextMuted
	}
}

// IsProfit returns true if the current value represents a profit
func (p *PnLGauge) IsProfit() bool {
	return p.value > 0
}

// IsLoss returns true if the current value represents a loss
func (p *PnLGauge) IsLoss() bool {
	return p.value < 0
}

// IsBreakEven returns true if the current value is zero
func (p *PnLGauge) IsBreakEven() bool {
	return p.value == 0
}

// GetArrow returns the appropriate arrow character for the current trend
func (p *PnLGauge) GetArrow() string {
	switch {
	case p.value >= p.profitThreshold:
		return "↗"
	case p.value <= p.lossThreshold:
		return "↘"
	case p.value > 0:
		return "↑"
	case p.value < 0:
		return "↓"
	default:
		return "→"
	}
}

// ViewCompact renders a compact version of the gauge
func (p *PnLGauge) ViewCompact() string {
	color := p.GetColor()
	arrow := p.GetArrow()

	valueText := fmt.Sprintf("%.1f", p.value)
	if p.showPercent {
		valueText += "%"
	}

	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(valueText + " " + arrow)
}

// ViewDetailed renders a detailed version with status text
func (p *PnLGauge) ViewDetailed() string {
	gauge := p.View()
	status := p.GetStatus()

	statusColor := p.GetColor()

	styledStatus := lipgloss.NewStyle().Foreground(statusColor).Render(status)

	return gauge + "\n" + styledStatus
}
