package component

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// TableColumn represents a column configuration
type TableColumn struct {
	Header string
	Width  int
	Align  lipgloss.Position
}

// TableRow represents a row of data
type TableRow struct {
	Data     []string
	Selected bool
	Style    lipgloss.Style
}

// Table represents a data table component
type Table struct {
	columns     []TableColumn
	rows        []TableRow
	width       int
	height      int
	selectedRow int

	// Styling
	headerStyle      lipgloss.Style
	rowStyle         lipgloss.Style
	selectedRowStyle lipgloss.Style
	borderStyle      lipgloss.Style

	// Configuration
	showBorder  bool
	showHeaders bool
	selectable  bool
	zebra       bool // Alternating row colors
}

// NewTable creates a new table component
func NewTable() *Table {
	palette := style.DefaultPalette()

	return &Table{
		columns:     make([]TableColumn, 0),
		rows:        make([]TableRow, 0),
		selectedRow: 0,

		headerStyle: lipgloss.NewStyle().
			Foreground(palette.Secondary).
			Bold(true).
			Padding(0, 1),

		rowStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(0, 1),

		selectedRowStyle: lipgloss.NewStyle().
			Foreground(palette.Background).
			Background(palette.Primary).
			Padding(0, 1),

		borderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.TextMuted),

		showBorder:  true,
		showHeaders: true,
		selectable:  true,
		zebra:       false,
	}
}

// SetColumns sets the table columns
func (t *Table) SetColumns(columns []TableColumn) *Table {
	t.columns = columns
	return t
}

// AddColumn adds a column to the table
func (t *Table) AddColumn(header string, width int, align lipgloss.Position) *Table {
	t.columns = append(t.columns, TableColumn{
		Header: header,
		Width:  width,
		Align:  align,
	})
	return t
}

// SetRows sets all table rows
func (t *Table) SetRows(rows [][]string) *Table {
	t.rows = make([]TableRow, len(rows))
	for i, rowData := range rows {
		t.rows[i] = TableRow{
			Data:     rowData,
			Selected: false,
			Style:    t.rowStyle,
		}
	}
	return t
}

// AddRow adds a row to the table
func (t *Table) AddRow(data []string) *Table {
	t.rows = append(t.rows, TableRow{
		Data:     data,
		Selected: false,
		Style:    t.rowStyle,
	})
	return t
}

// SetRowStyle sets a custom style for a specific row
func (t *Table) SetRowStyle(rowIndex int, style lipgloss.Style) *Table {
	if rowIndex >= 0 && rowIndex < len(t.rows) {
		t.rows[rowIndex].Style = style
	}
	return t
}

// SetSize sets the table dimensions
func (t *Table) SetSize(width, height int) *Table {
	t.width = width
	t.height = height
	return t
}

// SetSelectedRow sets the currently selected row
func (t *Table) SetSelectedRow(index int) *Table {
	if index >= 0 && index < len(t.rows) {
		t.selectedRow = index
	}
	return t
}

// GetSelectedRow returns the currently selected row index
func (t *Table) GetSelectedRow() int {
	return t.selectedRow
}

// MoveUp moves selection up
func (t *Table) MoveUp() *Table {
	if t.selectable && t.selectedRow > 0 {
		t.selectedRow--
	}
	return t
}

// MoveDown moves selection down
func (t *Table) MoveDown() *Table {
	if t.selectable && t.selectedRow < len(t.rows)-1 {
		t.selectedRow++
	}
	return t
}

// SetSelectable enables/disables row selection
func (t *Table) SetSelectable(selectable bool) *Table {
	t.selectable = selectable
	return t
}

// SetShowBorder enables/disables table border
func (t *Table) SetShowBorder(show bool) *Table {
	t.showBorder = show
	return t
}

// SetShowHeaders enables/disables column headers
func (t *Table) SetShowHeaders(show bool) *Table {
	t.showHeaders = show
	return t
}

// SetZebra enables/disables alternating row colors
func (t *Table) SetZebra(zebra bool) *Table {
	t.zebra = zebra
	return t
}

// View renders the table
func (t *Table) View() string {
	if len(t.columns) == 0 {
		return "No columns defined"
	}

	var content strings.Builder

	// Calculate column widths if not set
	t.calculateColumnWidths()

	// Render headers
	if t.showHeaders {
		var headerRow strings.Builder
		for i, col := range t.columns {
			cell := t.renderCell(col.Header, col.Width, col.Align, t.headerStyle)
			headerRow.WriteString(cell)

			if i < len(t.columns)-1 {
				headerRow.WriteString("│")
			}
		}
		content.WriteString(headerRow.String())
		content.WriteString("\n")

		// Header separator
		var separator strings.Builder
		for i, col := range t.columns {
			separator.WriteString(strings.Repeat("─", col.Width))
			if i < len(t.columns)-1 {
				separator.WriteString("┼")
			}
		}
		content.WriteString(separator.String())
		content.WriteString("\n")
	}

	// Render rows
	for rowIndex, row := range t.rows {
		var rowStr strings.Builder

		// Determine row style
		rowStyle := row.Style
		if t.selectable && rowIndex == t.selectedRow {
			rowStyle = t.selectedRowStyle
		} else if t.zebra && rowIndex%2 == 1 {
			// Alternate row style for zebra striping
			palette := style.DefaultPalette()
			rowStyle = rowStyle.Background(palette.BackgroundAlt)
		}

		for i, col := range t.columns {
			cellData := ""
			if i < len(row.Data) {
				cellData = row.Data[i]
			}

			cell := t.renderCell(cellData, col.Width, col.Align, rowStyle)
			rowStr.WriteString(cell)

			if i < len(t.columns)-1 {
				rowStr.WriteString("│")
			}
		}

		content.WriteString(rowStr.String())
		if rowIndex < len(t.rows)-1 {
			content.WriteString("\n")
		}
	}

	result := content.String()

	// Apply border if enabled
	if t.showBorder {
		result = t.borderStyle.Render(result)
	}

	return result
}

// renderCell renders a single table cell
func (t *Table) renderCell(content string, width int, align lipgloss.Position, style lipgloss.Style) string {
	// Truncate content if it's too long
	if len(content) > width {
		if width > 3 {
			content = content[:width-3] + "..."
		} else {
			content = content[:width]
		}
	}

	// Apply alignment and padding
	cellStyle := style.Width(width).Align(align)
	return cellStyle.Render(content)
}

// calculateColumnWidths calculates column widths if not explicitly set
func (t *Table) calculateColumnWidths() {
	if t.width <= 0 {
		return
	}

	// Count columns with explicit widths
	totalExplicitWidth := 0
	autoWidthColumns := 0

	for _, col := range t.columns {
		if col.Width > 0 {
			totalExplicitWidth += col.Width
		} else {
			autoWidthColumns++
		}
	}

	// Calculate available width for auto-width columns
	separatorWidth := len(t.columns) - 1 // For column separators
	availableWidth := t.width - totalExplicitWidth - separatorWidth

	if autoWidthColumns > 0 && availableWidth > 0 {
		autoWidth := availableWidth / autoWidthColumns

		// Set auto widths
		for i := range t.columns {
			if t.columns[i].Width <= 0 {
				t.columns[i].Width = autoWidth
			}
		}
	}
}

// GetRowCount returns the number of rows
func (t *Table) GetRowCount() int {
	return len(t.rows)
}

// GetSelectedRowData returns the data of the currently selected row
func (t *Table) GetSelectedRowData() []string {
	if t.selectedRow >= 0 && t.selectedRow < len(t.rows) {
		return t.rows[t.selectedRow].Data
	}
	return nil
}

// Clear removes all rows from the table
func (t *Table) Clear() *Table {
	t.rows = make([]TableRow, 0)
	t.selectedRow = 0
	return t
}

// UpdateRow updates a specific row's data
func (t *Table) UpdateRow(index int, data []string) *Table {
	if index >= 0 && index < len(t.rows) {
		t.rows[index].Data = data
	}
	return t
}
