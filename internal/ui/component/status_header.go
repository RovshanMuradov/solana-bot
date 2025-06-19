package component

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/state"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// RPCStatus represents the current RPC connection status
type RPCStatus struct {
	Connected bool
	Latency   time.Duration
	LastCheck time.Time
}

// StatusHeader provides a clean header with essential status information
type StatusHeader struct {
	wallet    string
	rpcStatus RPCStatus
	totalPnL  float64
	style     StatusHeaderStyle
	width     int
}

// StatusHeaderStyle contains all styling for the status header
type StatusHeaderStyle struct {
	container   lipgloss.Style
	title       lipgloss.Style
	wallet      lipgloss.Style
	rpcGood     lipgloss.Style
	rpcBad      lipgloss.Style
	pnlPositive lipgloss.Style
	pnlNegative lipgloss.Style
	pnlNeutral  lipgloss.Style
}

// NewStatusHeader creates a new status header component
func NewStatusHeader() *StatusHeader {
	palette := style.DefaultPalette()

	return &StatusHeader{
		wallet:   "Unknown",
		totalPnL: 0.0,
		style: StatusHeaderStyle{
			container: lipgloss.NewStyle().
				Background(palette.Background).
				Foreground(palette.Text).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(palette.Primary).
				Padding(0, 2).
				MarginBottom(1),

			title: lipgloss.NewStyle().
				Foreground(palette.Primary).
				Bold(true),

			wallet: lipgloss.NewStyle().
				Foreground(palette.TextSecondary).
				Bold(false),

			rpcGood: lipgloss.NewStyle().
				Foreground(palette.Success).
				Bold(true),

			rpcBad: lipgloss.NewStyle().
				Foreground(palette.Error).
				Bold(true),

			pnlPositive: lipgloss.NewStyle().
				Foreground(palette.Success).
				Bold(true),

			pnlNegative: lipgloss.NewStyle().
				Foreground(palette.Error).
				Bold(true),

			pnlNeutral: lipgloss.NewStyle().
				Foreground(palette.TextMuted).
				Bold(false),
		},
	}
}

// SetWallet updates the wallet address display
func (sh *StatusHeader) SetWallet(wallet string) {
	if len(wallet) > 8 {
		sh.wallet = wallet[:8] + "..."
	} else {
		sh.wallet = wallet
	}
}

// SetRPCStatus updates the RPC connection status
func (sh *StatusHeader) SetRPCStatus(status RPCStatus) {
	sh.rpcStatus = status
}

// SetTotalPnL updates the total PnL display
func (sh *StatusHeader) SetTotalPnL(pnl float64) {
	sh.totalPnL = pnl
}

// SetWidth sets the component width for responsive layout
func (sh *StatusHeader) SetWidth(width int) {
	sh.width = width
	sh.style.container = sh.style.container.Width(width - 4)
}

// Update recalculates total PnL from global cache
func (sh *StatusHeader) Update() {
	if state.GlobalCache != nil {
		positions := state.GlobalCache.GetActivePositions()
		totalPnL := 0.0
		for _, pos := range positions {
			totalPnL += pos.PnL
		}
		sh.totalPnL = totalPnL
	}
}

// View renders the status header
func (sh *StatusHeader) View() string {
	title := sh.style.title.Render("Solana Bot v1.0")
	wallet := sh.style.wallet.Render(fmt.Sprintf("Wallet: %s", sh.wallet))
	rpcStatus := sh.renderRPCStatus()
	pnlStatus := sh.renderPnLStatus()

	content := lipgloss.JoinHorizontal(
		lipgloss.Left,
		title,
		" | ",
		wallet,
		" | ",
		rpcStatus,
		" | ",
		pnlStatus,
	)

	return sh.style.container.Render(content)
}

// renderRPCStatus renders the RPC connection status with emoji
func (sh *StatusHeader) renderRPCStatus() string {
	if sh.rpcStatus.Connected {
		latencyMs := sh.rpcStatus.Latency.Milliseconds()
		status := fmt.Sprintf("🟢 RPC: OK (%dms)", latencyMs)
		return sh.style.rpcGood.Render(status)
	}

	status := "🔴 RPC: Disconnected"
	return sh.style.rpcBad.Render(status)
}

// renderPnLStatus renders the total PnL with trend emoji
func (sh *StatusHeader) renderPnLStatus() string {
	var emoji string
	var renderer lipgloss.Style

	if sh.totalPnL > 0 {
		emoji = "📈"
		renderer = sh.style.pnlPositive
	} else if sh.totalPnL < 0 {
		emoji = "📉"
		renderer = sh.style.pnlNegative
	} else {
		emoji = ""
		renderer = sh.style.pnlNeutral
	}

	var status string
	if emoji != "" {
		status = fmt.Sprintf("Total PnL: %.4f SOL %s", sh.totalPnL, emoji)
	} else {
		status = fmt.Sprintf("Total PnL: %.4f SOL", sh.totalPnL)
	}

	return renderer.Render(status)
}

// GetHeight returns the component height for layout calculations
func (sh *StatusHeader) GetHeight() int {
	return 3 // Border + padding + content
}
