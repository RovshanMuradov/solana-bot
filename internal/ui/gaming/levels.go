package gaming

import (
	"math"

	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// GamingLevel represents a trading proficiency level
type GamingLevel struct {
	Level       int     `json:"level"`
	Badge       string  `json:"badge"`
	Title       string  `json:"title"`
	MinPnL      float64 `json:"min_pnl"`     // Minimum PnL to achieve this level
	MaxPnL      float64 `json:"max_pnl"`     // Maximum PnL for this level (-1 for unlimited)
	Color       string  `json:"color"`       // Color for badge display
	Description string  `json:"description"` // Level description
}

// TradingLevels defines all available trading levels
var TradingLevels = []GamingLevel{
	{
		Level:       1,
		Badge:       "L1",
		Title:       "Rookie",
		MinPnL:      -math.Inf(1), // No minimum (can start with losses)
		MaxPnL:      0.0,
		Color:       "#6C7280", // Base01 - muted for beginners
		Description: "Learning the ropes",
	},
	{
		Level:       3,
		Badge:       "L3",
		Title:       "Trader",
		MinPnL:      0.001, // Small positive PnL
		MaxPnL:      0.1,
		Color:       "#FFB500", // Yellow - getting better
		Description: "Starting to profit",
	},
	{
		Level:       5,
		Badge:       "L5",
		Title:       "Pro",
		MinPnL:      0.1,
		MaxPnL:      0.5,
		Color:       "#3B82F6", // Blue - competent
		Description: "Consistent performance",
	},
	{
		Level:       10,
		Badge:       "Pro",
		Title:       "Master",
		MinPnL:      0.5,
		MaxPnL:      2.0,
		Color:       "#8B5CF6", // Purple - advanced
		Description: "Advanced trading skills",
	},
	{
		Level:       20,
		Badge:       "Legend",
		Title:       "Legend",
		MinPnL:      2.0,
		MaxPnL:      -1,        // Unlimited
		Color:       "#2AFFAA", // Green - legendary
		Description: "Trading mastery achieved",
	},
}

// CalculateLevel determines the trading level based on total PnL
func CalculateLevel(totalPnL float64) GamingLevel {
	// Find the highest level that the trader qualifies for
	currentLevel := TradingLevels[0] // Default to rookie

	for _, level := range TradingLevels {
		if totalPnL >= level.MinPnL && (level.MaxPnL == -1 || totalPnL <= level.MaxPnL) {
			currentLevel = level
		}
	}

	return currentLevel
}

// GetLevelByBadge returns a level by its badge string
func GetLevelByBadge(badge string) (GamingLevel, bool) {
	for _, level := range TradingLevels {
		if level.Badge == badge {
			return level, true
		}
	}
	return GamingLevel{}, false
}

// GetNextLevel returns the next level to achieve, if any
func GetNextLevel(currentLevel GamingLevel) (GamingLevel, bool) {
	for i, level := range TradingLevels {
		if level.Level == currentLevel.Level && i < len(TradingLevels)-1 {
			return TradingLevels[i+1], true
		}
	}
	return GamingLevel{}, false // Already at max level
}

// GetProgressToNext calculates progress percentage to the next level
func GetProgressToNext(totalPnL float64, currentLevel GamingLevel) float64 {
	nextLevel, hasNext := GetNextLevel(currentLevel)
	if !hasNext || nextLevel.MaxPnL == -1 {
		return 100.0 // At max level
	}

	if totalPnL <= currentLevel.MinPnL {
		return 0.0
	}

	rangeSize := nextLevel.MinPnL - currentLevel.MinPnL
	progress := totalPnL - currentLevel.MinPnL

	if rangeSize <= 0 {
		return 100.0
	}

	percentage := (progress / rangeSize) * 100
	if percentage > 100 {
		percentage = 100
	} else if percentage < 0 {
		percentage = 0
	}

	return percentage
}

// LevelBadgeStyle provides styling for level badges
type LevelBadgeStyle struct {
	badge       lipgloss.Style
	title       lipgloss.Style
	description lipgloss.Style
}

// NewLevelBadgeStyle creates a styled badge for a gaming level
func NewLevelBadgeStyle(level GamingLevel) LevelBadgeStyle {
	palette := style.DefaultPalette()

	return LevelBadgeStyle{
		badge: lipgloss.NewStyle().
			Foreground(lipgloss.Color(level.Color)).
			Background(palette.BackgroundAlt).
			Bold(true).
			Padding(0, 1).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(level.Color)),

		title: lipgloss.NewStyle().
			Foreground(lipgloss.Color(level.Color)).
			Bold(true),

		description: lipgloss.NewStyle().
			Foreground(palette.TextMuted).
			Italic(true),
	}
}

// RenderBadge renders just the badge (e.g., "L3", "Pro")
func (lbs LevelBadgeStyle) RenderBadge(level GamingLevel) string {
	return lbs.badge.Render(level.Badge)
}

// RenderTitle renders just the title (e.g., "Trader", "Master")
func (lbs LevelBadgeStyle) RenderTitle(level GamingLevel) string {
	return lbs.title.Render(level.Title)
}

// RenderFull renders badge + title (e.g., "L3 Trader")
func (lbs LevelBadgeStyle) RenderFull(level GamingLevel) string {
	badge := lbs.badge.Render(level.Badge)
	title := lbs.title.Render(level.Title)
	return lipgloss.JoinHorizontal(lipgloss.Left, badge, " ", title)
}

// RenderWithDescription renders full info with description
func (lbs LevelBadgeStyle) RenderWithDescription(level GamingLevel) string {
	full := lbs.RenderFull(level)
	desc := lbs.description.Render(level.Description)
	return lipgloss.JoinVertical(lipgloss.Left, full, desc)
}

// GetAllLevels returns all available levels for UI display
func GetAllLevels() []GamingLevel {
	return TradingLevels
}

// GetLevelStats returns statistics about level distribution
func GetLevelStats() map[string]int {
	stats := make(map[string]int)
	for _, level := range TradingLevels {
		stats[level.Badge] = 0 // Initialize counts
	}
	return stats
}
