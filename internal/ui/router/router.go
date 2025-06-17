package router

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
)

// Screen represents a screen that can be navigated to
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
	SetSize(width, height int)
}

// Router manages navigation between screens using a stack-based approach
type Router struct {
	stack  []Screen
	width  int
	height int
}

// New creates a new router with the initial screen
func New(initialScreen Screen) *Router {
	return &Router{
		stack: []Screen{initialScreen},
	}
}

// Init initializes the router
func (r *Router) Init() tea.Cmd {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1].Init()
}

// Update processes messages and updates the current screen
func (r *Router) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle router-specific messages
	switch msg := msg.(type) {
	case ui.RouterMsg:
		return r, r.Navigate(msg.To)

	case tea.WindowSizeMsg:
		r.SetSize(msg.Width, msg.Height)
		// Forward size to current screen
		if len(r.stack) > 0 {
			r.stack[len(r.stack)-1].SetSize(msg.Width, msg.Height)
		}
		return r, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Handle back navigation on Escape
			if len(r.stack) > 1 {
				return r, r.Back()
			}
		}
	}

	// Update current screen
	if len(r.stack) > 0 {
		currentScreen := r.stack[len(r.stack)-1]
		updatedScreen, cmd := currentScreen.Update(msg)
		r.stack[len(r.stack)-1] = updatedScreen

		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return r, tea.Batch(cmds...)
}

// View renders the current screen
func (r *Router) View() string {
	if len(r.stack) == 0 {
		return "No screen available"
	}
	return r.stack[len(r.stack)-1].View()
}

// SetSize sets the size for the router and current screen
func (r *Router) SetSize(width, height int) {
	r.width = width
	r.height = height

	// Set size for current screen
	if len(r.stack) > 0 {
		r.stack[len(r.stack)-1].SetSize(width, height)
	}
}

// Push adds a new screen to the navigation stack
func (r *Router) Push(screen Screen) tea.Cmd {
	screen.SetSize(r.width, r.height)
	r.stack = append(r.stack, screen)
	return screen.Init()
}

// Pop removes the current screen from the stack
func (r *Router) Pop() tea.Cmd {
	if len(r.stack) <= 1 {
		return nil // Can't pop the last screen
	}

	r.stack = r.stack[:len(r.stack)-1]

	// Re-initialize the current screen
	if len(r.stack) > 0 {
		currentScreen := r.stack[len(r.stack)-1]
		currentScreen.SetSize(r.width, r.height)
		return currentScreen.Init()
	}

	return nil
}

// Replace replaces the current screen with a new one
func (r *Router) Replace(screen Screen) tea.Cmd {
	if len(r.stack) == 0 {
		return r.Push(screen)
	}

	screen.SetSize(r.width, r.height)
	r.stack[len(r.stack)-1] = screen
	return screen.Init()
}

// Navigate is a helper method that creates a RouterMsg command
func (r *Router) Navigate(route ui.Route) tea.Cmd {
	return func() tea.Msg {
		return ui.RouterMsg{To: route}
	}
}

// Back navigates back to the previous screen
func (r *Router) Back() tea.Cmd {
	return r.Pop()
}

// Current returns the current screen
func (r *Router) Current() Screen {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1]
}

// Depth returns the current navigation depth
func (r *Router) Depth() int {
	return len(r.stack)
}

// Clear removes all screens except the first one
func (r *Router) Clear() tea.Cmd {
	if len(r.stack) <= 1 {
		return nil
	}

	r.stack = r.stack[:1]

	// Re-initialize the root screen
	if len(r.stack) > 0 {
		currentScreen := r.stack[0]
		currentScreen.SetSize(r.width, r.height)
		return currentScreen.Init()
	}

	return nil
}

// CanGoBack returns true if there are screens to go back to
func (r *Router) CanGoBack() bool {
	return len(r.stack) > 1
}
