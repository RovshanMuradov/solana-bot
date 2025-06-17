package component

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// Sparkline represents a mini graph component for showing price trends
type Sparkline struct {
	data     []float64
	width    int
	style    lipgloss.Style
	color    lipgloss.Color
	showText bool
}

// NewSparkline creates a new sparkline component
func NewSparkline(width int) *Sparkline {
	return &Sparkline{
		data:     make([]float64, 0),
		width:    width,
		style:    lipgloss.NewStyle(),
		color:    style.DefaultPalette().Primary,
		showText: false,
	}
}

// SetData sets the data points for the sparkline
func (s *Sparkline) SetData(data []float64) *Sparkline {
	s.data = make([]float64, len(data))
	copy(s.data, data)
	return s
}

// AddDataPoint adds a new data point to the sparkline
func (s *Sparkline) AddDataPoint(value float64) *Sparkline {
	s.data = append(s.data, value)
	// Keep only the last `width` points
	if len(s.data) > s.width {
		s.data = s.data[len(s.data)-s.width:]
	}
	return s
}

// SetWidth sets the width of the sparkline
func (s *Sparkline) SetWidth(width int) *Sparkline {
	s.width = width
	// Trim data if necessary
	if len(s.data) > width {
		s.data = s.data[len(s.data)-width:]
	}
	return s
}

// SetStyle sets the lipgloss style for the sparkline
func (s *Sparkline) SetStyle(style lipgloss.Style) *Sparkline {
	s.style = style
	return s
}

// SetColor sets the color for the sparkline
func (s *Sparkline) SetColor(color lipgloss.Color) *Sparkline {
	s.color = color
	return s
}

// ShowText enables/disables text display alongside the sparkline
func (s *Sparkline) ShowText(show bool) *Sparkline {
	s.showText = show
	return s
}

// View renders the sparkline
func (s *Sparkline) View() string {
	if len(s.data) == 0 {
		return s.style.Render(strings.Repeat("▁", s.width))
	}

	// Create the sparkline characters
	blocks := s.generateSparkBlocks()

	// Apply color styling
	styledBlocks := s.style.Foreground(s.color).Render(blocks)

	if s.showText && len(s.data) > 0 {
		// Add current value and trend information
		current := s.data[len(s.data)-1]
		var trend string
		var trendColor lipgloss.Color

		if len(s.data) >= 2 {
			prev := s.data[len(s.data)-2]
			if current > prev {
				trend = "↗"
				trendColor = style.DefaultPalette().Success
			} else if current < prev {
				trend = "↘"
				trendColor = style.DefaultPalette().Error
			} else {
				trend = "→"
				trendColor = style.DefaultPalette().TextMuted
			}
		}

		trendStyled := lipgloss.NewStyle().Foreground(trendColor).Render(trend)
		return styledBlocks + " " + trendStyled
	}

	return styledBlocks
}

// generateSparkBlocks creates the spark characters based on data
func (s *Sparkline) generateSparkBlocks() string {
	if len(s.data) == 0 {
		return strings.Repeat("▁", s.width)
	}

	// Find min and max values for normalization
	min, max := s.getMinMax()

	// If all values are the same, show a flat line
	if min == max {
		return strings.Repeat("▄", minInt(len(s.data), s.width))
	}

	// Spark characters from lowest to highest
	sparkChars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	var result strings.Builder

	// Generate characters for each data point
	for i, value := range s.data {
		if i >= s.width {
			break
		}

		// Normalize value to 0-7 range for spark characters
		normalized := (value - min) / (max - min)
		index := int(normalized * float64(len(sparkChars)-1))

		// Ensure index is within bounds
		if index < 0 {
			index = 0
		} else if index >= len(sparkChars) {
			index = len(sparkChars) - 1
		}

		result.WriteRune(sparkChars[index])
	}

	// Pad with spaces if we have fewer data points than width
	for result.Len() < s.width {
		result.WriteRune(' ')
	}

	return result.String()
}

// getMinMax finds the minimum and maximum values in the data
func (s *Sparkline) getMinMax() (float64, float64) {
	if len(s.data) == 0 {
		return 0, 0
	}

	min := s.data[0]
	max := s.data[0]

	for _, value := range s.data {
		if value < min {
			min = value
		}
		if value > max {
			max = value
		}
	}

	return min, max
}

// GetTrend returns the overall trend of the data
func (s *Sparkline) GetTrend() string {
	if len(s.data) < 2 {
		return "→"
	}

	first := s.data[0]
	last := s.data[len(s.data)-1]

	change := (last - first) / first * 100

	if math.Abs(change) < 0.1 {
		return "→" // Flat
	} else if change > 0 {
		return "↗" // Up
	} else {
		return "↘" // Down
	}
}

// GetChangePercent returns the percentage change from first to last data point
func (s *Sparkline) GetChangePercent() float64 {
	if len(s.data) < 2 {
		return 0
	}

	first := s.data[0]
	last := s.data[len(s.data)-1]

	if first == 0 {
		return 0
	}

	return (last - first) / first * 100
}

// Clear removes all data points
func (s *Sparkline) Clear() *Sparkline {
	s.data = make([]float64, 0)
	return s
}

// minInt helper function
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
