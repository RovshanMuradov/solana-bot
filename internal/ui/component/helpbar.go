package component

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// HelpBar represents a help bar component showing keyboard shortcuts
type HelpBar struct {
	keyBindings []key.Binding
	width       int

	// Styling
	keyStyle       lipgloss.Style
	descStyle      lipgloss.Style
	sepStyle       lipgloss.Style
	containerStyle lipgloss.Style

	// Configuration
	compact    bool
	showBorder bool
}

// NewHelpBar creates a new help bar component
func NewHelpBar() *HelpBar {
	palette := style.DefaultPalette()

	return &HelpBar{
		keyBindings: make([]key.Binding, 0),
		width:       80,

		keyStyle: lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true),

		descStyle: lipgloss.NewStyle().
			Foreground(palette.TextMuted),

		sepStyle: lipgloss.NewStyle().
			Foreground(palette.TextMuted),

		containerStyle: lipgloss.NewStyle().
			Padding(0, 1).
			Margin(1, 0, 0, 0),

		compact:    false,
		showBorder: false,
	}
}

// SetKeyBindings sets the key bindings to display
func (h *HelpBar) SetKeyBindings(bindings []key.Binding) *HelpBar {
	h.keyBindings = bindings
	return h
}

// SetWidth sets the help bar width
func (h *HelpBar) SetWidth(width int) *HelpBar {
	h.width = width
	return h
}

// SetCompact enables/disables compact mode
func (h *HelpBar) SetCompact(compact bool) *HelpBar {
	h.compact = compact
	return h
}

// SetShowBorder enables/disables border display
func (h *HelpBar) SetShowBorder(show bool) *HelpBar {
	h.showBorder = show
	if show {
		h.containerStyle = h.containerStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(style.DefaultPalette().TextMuted)
	} else {
		h.containerStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Margin(1, 0, 0, 0)
	}
	return h
}

// View renders the help bar
func (h *HelpBar) View() string {
	if len(h.keyBindings) == 0 {
		return ""
	}

	var helpItems []string
	availableWidth := h.width - 4 // Account for padding

	if h.compact {
		helpItems = h.renderCompact(availableWidth)
	} else {
		helpItems = h.renderFull(availableWidth)
	}

	separator := h.sepStyle.Render(" • ")
	content := strings.Join(helpItems, separator)

	// Wrap content if it's too long
	if len(content) > availableWidth {
		content = h.wrapContent(helpItems, availableWidth, separator)
	}

	return h.containerStyle.Width(h.width).Render(content)
}

// renderCompact renders help items in compact mode (keys only)
func (h *HelpBar) renderCompact(maxWidth int) []string {
	items := make([]string, 0, len(h.keyBindings))
	currentWidth := 0

	for _, binding := range h.keyBindings {
		if !binding.Enabled() {
			continue
		}

		keys := binding.Keys()
		if len(keys) == 0 {
			continue
		}

		// Use the first key for compact mode
		keyText := h.keyStyle.Render(keys[0])
		itemWidth := lipgloss.Width(keyText) + 3 // 3 for separator

		if currentWidth+itemWidth > maxWidth && len(items) > 0 {
			break
		}

		items = append(items, keyText)
		currentWidth += itemWidth
	}

	return items
}

// renderFull renders help items in full mode (keys + descriptions)
func (h *HelpBar) renderFull(maxWidth int) []string {
	items := make([]string, 0, len(h.keyBindings))
	currentWidth := 0

	for _, binding := range h.keyBindings {
		if !binding.Enabled() {
			continue
		}

		keys := binding.Keys()
		help := binding.Help()
		if len(keys) == 0 || help.Desc == "" {
			continue
		}

		// Format as "key description"
		keyText := h.keyStyle.Render(keys[0])
		descText := h.descStyle.Render(help.Desc)

		item := keyText + " " + descText
		itemWidth := lipgloss.Width(item) + 3 // 3 for separator

		if currentWidth+itemWidth > maxWidth && len(items) > 0 {
			break
		}

		items = append(items, item)
		currentWidth += itemWidth
	}

	return items
}

// wrapContent wraps content to fit within the available width
func (h *HelpBar) wrapContent(items []string, maxWidth int, separator string) string {
	var lines []string
	var currentLine []string
	currentWidth := 0
	sepWidth := lipgloss.Width(separator)

	for _, item := range items {
		itemWidth := lipgloss.Width(item) + sepWidth

		if currentWidth+itemWidth > maxWidth && len(currentLine) > 0 {
			// Start a new line
			lines = append(lines, strings.Join(currentLine, separator))
			currentLine = []string{item}
			currentWidth = itemWidth
		} else {
			currentLine = append(currentLine, item)
			currentWidth += itemWidth
		}
	}

	// Add the last line
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, separator))
	}

	return strings.Join(lines, "\n")
}

// ViewContextual renders contextual help for specific key bindings
func (h *HelpBar) ViewContextual(bindings []key.Binding) string {
	originalBindings := h.keyBindings
	h.SetKeyBindings(bindings)
	result := h.View()
	h.SetKeyBindings(originalBindings)
	return result
}

// ViewQuick renders a quick help with only the most important shortcuts
func (h *HelpBar) ViewQuick() string {
	if len(h.keyBindings) == 0 {
		return ""
	}

	// Show only the first few most important shortcuts
	maxItems := 4
	if h.width < 60 {
		maxItems = 2
	}

	quickItems := make([]string, 0, maxItems)
	count := 0

	for _, binding := range h.keyBindings {
		if !binding.Enabled() || count >= maxItems {
			break
		}

		keys := binding.Keys()
		help := binding.Help()
		if len(keys) == 0 || help.Desc == "" {
			continue
		}

		keyText := h.keyStyle.Render(keys[0])
		descText := h.descStyle.Render(help.Desc)

		quickItems = append(quickItems, keyText+" "+descText)
		count++
	}

	separator := h.sepStyle.Render(" • ")
	content := strings.Join(quickItems, separator)
	return h.containerStyle.Width(h.width).Render(content)
}

// ViewNavigationOnly renders only navigation-related shortcuts
func (h *HelpBar) ViewNavigationOnly() string {
	var navBindings []key.Binding

	// Filter for navigation keys
	navKeys := []string{"up", "down", "left", "right", "enter", "esc", "tab", "q"}

	for _, binding := range h.keyBindings {
		if !binding.Enabled() {
			continue
		}

		keys := binding.Keys()
		if len(keys) == 0 {
			continue
		}

		// Check if this is a navigation key
		for _, navKey := range navKeys {
			for _, key := range keys {
				if strings.Contains(key, navKey) {
					navBindings = append(navBindings, binding)
					break
				}
			}
		}
	}

	return h.ViewContextual(navBindings)
}

// Clear clears all key bindings
func (h *HelpBar) Clear() *HelpBar {
	h.keyBindings = make([]key.Binding, 0)
	return h
}

// AddKeyBinding adds a single key binding
func (h *HelpBar) AddKeyBinding(binding key.Binding) *HelpBar {
	h.keyBindings = append(h.keyBindings, binding)
	return h
}

// AddKeyBindings adds multiple key bindings
func (h *HelpBar) AddKeyBindings(bindings []key.Binding) *HelpBar {
	h.keyBindings = append(h.keyBindings, bindings...)
	return h
}
