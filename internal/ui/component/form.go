package component

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// FieldType represents the type of form field
type FieldType int

const (
	FieldTypeText FieldType = iota
	FieldTypeNumber
	FieldTypePassword
	FieldTypeTextArea
	FieldTypeSelect
	FieldTypeCheckbox
)

// Legacy aliases for backward compatibility
const (
	TextInput     = FieldTypeText
	NumberInput   = FieldTypeNumber
	PasswordInput = FieldTypePassword
	TextArea      = FieldTypeTextArea
	Select        = FieldTypeSelect
	Checkbox      = FieldTypeCheckbox
)

// FormField represents a single form field
type FormField struct {
	Name        string
	Label       string
	Type        FieldType
	Value       string
	Options     []string // For select fields
	Placeholder string
	Required    bool
	Validation  func(string) error
	Error       string

	// Internal state
	textInput   textinput.Model
	focused     bool
	selectedIdx int // For select fields
}

// Form represents a form component with multiple fields
type Form struct {
	fields     []FormField
	focusIndex int
	width      int
	height     int

	// Styling
	labelStyle    lipgloss.Style
	inputStyle    lipgloss.Style
	focusedStyle  lipgloss.Style
	errorStyle    lipgloss.Style
	checkboxStyle lipgloss.Style

	// State
	submitted bool
}

// NewForm creates a new form component
func NewForm() *Form {
	palette := style.DefaultPalette()

	return &Form{
		fields:     make([]FormField, 0),
		focusIndex: 0,

		labelStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Bold(true).
			MarginRight(1),

		inputStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Background(palette.BackgroundAlt).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.TextMuted),

		focusedStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Background(palette.BackgroundAlt).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Primary),

		errorStyle: lipgloss.NewStyle().
			Foreground(palette.Error).
			MarginTop(1),

		checkboxStyle: lipgloss.NewStyle().
			Foreground(palette.Primary),
	}
}

// AddField adds a field to the form
func (f *Form) AddField(name string, fieldType FieldType, label string, required bool, placeholder string) *Form {
	ti := textinput.New()
	ti.Width = 40
	ti.Placeholder = placeholder

	switch fieldType {
	case PasswordInput:
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	case NumberInput:
		if placeholder == "" {
			ti.Placeholder = "0"
		}
	}

	field := FormField{
		Name:        name,
		Label:       label,
		Type:        fieldType,
		Value:       "",
		Options:     make([]string, 0),
		Placeholder: placeholder,
		Required:    required,
		textInput:   ti,
		focused:     false,
	}

	f.fields = append(f.fields, field)

	// Focus first field
	if len(f.fields) == 1 {
		f.fields[0].focused = true
		f.fields[0].textInput.Focus()
	}

	return f
}

// SetFieldValue sets the value of a field
func (f *Form) SetFieldValue(name, value string) *Form {
	for i := range f.fields {
		if f.fields[i].Name == name {
			f.fields[i].Value = value
			f.fields[i].textInput.SetValue(value)
			break
		}
	}
	return f
}

// SetFieldPlaceholder sets the placeholder for a field
func (f *Form) SetFieldPlaceholder(name, placeholder string) *Form {
	for i := range f.fields {
		if f.fields[i].Name == name {
			f.fields[i].Placeholder = placeholder
			f.fields[i].textInput.Placeholder = placeholder
			break
		}
	}
	return f
}

// SetFieldOptions sets options for select fields
func (f *Form) SetFieldOptions(name string, options []string) *Form {
	for i := range f.fields {
		if f.fields[i].Name == name && f.fields[i].Type == Select {
			f.fields[i].Options = options
			if len(options) > 0 {
				f.fields[i].Value = options[0]
			}
			break
		}
	}
	return f
}

// SetSelectOptions is an alias for SetFieldOptions for backward compatibility
func (f *Form) SetSelectOptions(name string, options []string) *Form {
	return f.SetFieldOptions(name, options)
}

// SetTitle sets the form title (placeholder for future use)
func (f *Form) SetTitle(title string) *Form {
	// For now, just return the form - title display can be added later
	return f
}

// SetWidth sets the form width
func (f *Form) SetWidth(width int) *Form {
	f.width = width
	// Update input width
	inputWidth := width - 4 // Account for padding and borders
	if inputWidth > 10 {
		for i := range f.fields {
			f.fields[i].textInput.Width = inputWidth
		}
	}
	return f
}

// GetFieldValue is an alias for GetValue for backward compatibility
func (f *Form) GetFieldValue(name string) string {
	return f.GetValue(name)
}

// Init initializes the form (for compatibility with tea.Model interface)
func (f *Form) Init() tea.Cmd {
	return nil
}

// SetFieldRequired sets whether a field is required
func (f *Form) SetFieldRequired(name string, required bool) *Form {
	for i := range f.fields {
		if f.fields[i].Name == name {
			f.fields[i].Required = required
			break
		}
	}
	return f
}

// SetFieldValidation sets a validation function for a field
func (f *Form) SetFieldValidation(name string, validation func(string) error) *Form {
	for i := range f.fields {
		if f.fields[i].Name == name {
			f.fields[i].Validation = validation
			break
		}
	}
	return f
}

// Update handles form input and updates
func (f *Form) Update(msg tea.Msg) (*Form, tea.Cmd) {
	if len(f.fields) == 0 {
		return f, nil
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			f.nextField()
		case "shift+tab":
			f.prevField()
		case "enter":
			if f.fields[f.focusIndex].Type == Select {
				// For select fields, Enter cycles through options
				f.nextSelectOption()
			} else {
				f.nextField()
			}
		case "up":
			if f.fields[f.focusIndex].Type == Select {
				f.prevSelectOption()
			}
		case "down":
			if f.fields[f.focusIndex].Type == Select {
				f.nextSelectOption()
			}
		case " ":
			if f.fields[f.focusIndex].Type == Checkbox {
				f.toggleCheckbox()
			}
		}
	}

	// Update the focused field
	if f.focusIndex < len(f.fields) {
		field := &f.fields[f.focusIndex]

		switch field.Type {
		case TextInput, NumberInput, PasswordInput, TextArea:
			var cmd tea.Cmd
			field.textInput, cmd = field.textInput.Update(msg)
			field.Value = field.textInput.Value()
			cmds = append(cmds, cmd)

			// Clear error when user types
			if field.Error != "" {
				field.Error = ""
			}
		}
	}

	return f, tea.Batch(cmds...)
}

// View renders the form
func (f *Form) View() string {
	if len(f.fields) == 0 {
		return "No fields defined"
	}

	var content strings.Builder

	for i, field := range f.fields {
		// Field label
		label := field.Label
		if field.Required {
			label += " *"
		}
		content.WriteString(f.labelStyle.Render(label))
		content.WriteString("\n")

		// Field input
		var fieldView string
		fieldStyle := f.inputStyle
		if i == f.focusIndex {
			fieldStyle = f.focusedStyle
		}

		switch field.Type {
		case TextInput, NumberInput, PasswordInput, TextArea:
			fieldView = fieldStyle.Render(field.textInput.View())

		case Select:
			selectedValue := ""
			if len(field.Options) > 0 && field.selectedIdx < len(field.Options) {
				selectedValue = field.Options[field.selectedIdx]
			}

			selectText := selectedValue
			if i == f.focusIndex {
				selectText += " ▼"
			}
			fieldView = fieldStyle.Render(selectText)

		case Checkbox:
			checkbox := "☐"
			if field.Value == "true" {
				checkbox = "☑"
			}

			checkboxText := checkbox + " " + field.Label
			if i == f.focusIndex {
				fieldView = f.focusedStyle.Render(checkboxText)
			} else {
				fieldView = f.checkboxStyle.Render(checkboxText)
			}
		}

		content.WriteString(fieldView)
		content.WriteString("\n")

		// Field error
		if field.Error != "" {
			content.WriteString(f.errorStyle.Render("⚠ " + field.Error))
			content.WriteString("\n")
		}

		// Add spacing between fields
		if i < len(f.fields)-1 {
			content.WriteString("\n")
		}
	}

	return content.String()
}

// nextField moves focus to the next field
func (f *Form) nextField() {
	if len(f.fields) == 0 {
		return
	}

	// Blur current field
	f.fields[f.focusIndex].focused = false
	f.fields[f.focusIndex].textInput.Blur()

	// Move to next field
	f.focusIndex = (f.focusIndex + 1) % len(f.fields)

	// Focus new field
	f.fields[f.focusIndex].focused = true
	if f.fields[f.focusIndex].Type == TextInput ||
		f.fields[f.focusIndex].Type == NumberInput ||
		f.fields[f.focusIndex].Type == PasswordInput ||
		f.fields[f.focusIndex].Type == TextArea {
		f.fields[f.focusIndex].textInput.Focus()
	}
}

// prevField moves focus to the previous field
func (f *Form) prevField() {
	if len(f.fields) == 0 {
		return
	}

	// Blur current field
	f.fields[f.focusIndex].focused = false
	f.fields[f.focusIndex].textInput.Blur()

	// Move to previous field
	f.focusIndex--
	if f.focusIndex < 0 {
		f.focusIndex = len(f.fields) - 1
	}

	// Focus new field
	f.fields[f.focusIndex].focused = true
	if f.fields[f.focusIndex].Type == TextInput ||
		f.fields[f.focusIndex].Type == NumberInput ||
		f.fields[f.focusIndex].Type == PasswordInput ||
		f.fields[f.focusIndex].Type == TextArea {
		f.fields[f.focusIndex].textInput.Focus()
	}
}

// nextSelectOption moves to the next option in a select field
func (f *Form) nextSelectOption() {
	field := &f.fields[f.focusIndex]
	if field.Type != Select || len(field.Options) == 0 {
		return
	}

	field.selectedIdx = (field.selectedIdx + 1) % len(field.Options)
	field.Value = field.Options[field.selectedIdx]
}

// prevSelectOption moves to the previous option in a select field
func (f *Form) prevSelectOption() {
	field := &f.fields[f.focusIndex]
	if field.Type != Select || len(field.Options) == 0 {
		return
	}

	field.selectedIdx--
	if field.selectedIdx < 0 {
		field.selectedIdx = len(field.Options) - 1
	}
	field.Value = field.Options[field.selectedIdx]
}

// toggleCheckbox toggles a checkbox field
func (f *Form) toggleCheckbox() {
	field := &f.fields[f.focusIndex]
	if field.Type != Checkbox {
		return
	}

	if field.Value == "true" {
		field.Value = "false"
	} else {
		field.Value = "true"
	}
}

// Validate validates all form fields
func (f *Form) Validate() bool {
	valid := true

	for i := range f.fields {
		field := &f.fields[i]

		// Clear previous errors
		field.Error = ""

		// Check required fields
		if field.Required && strings.TrimSpace(field.Value) == "" {
			field.Error = "This field is required"
			valid = false
			continue
		}

		// Run custom validation
		if field.Validation != nil {
			if err := field.Validation(field.Value); err != nil {
				field.Error = err.Error()
				valid = false
			}
		}
	}

	return valid
}

// GetValues returns all form field values as a map
func (f *Form) GetValues() map[string]string {
	values := make(map[string]string)
	for _, field := range f.fields {
		values[field.Name] = field.Value
	}
	return values
}

// GetValue returns the value of a specific field
func (f *Form) GetValue(name string) string {
	for _, field := range f.fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}

// Reset clears all form fields
func (f *Form) Reset() *Form {
	for i := range f.fields {
		f.fields[i].Value = ""
		f.fields[i].Error = ""
		f.fields[i].textInput.SetValue("")
		f.fields[i].selectedIdx = 0
	}

	f.focusIndex = 0
	if len(f.fields) > 0 {
		f.fields[0].focused = true
		f.fields[0].textInput.Focus()
	}

	return f
}

// SetSize sets the form dimensions
func (f *Form) SetSize(width, height int) *Form {
	f.width = width
	f.height = height

	// Update input width
	inputWidth := width - 4 // Account for padding and borders
	if inputWidth > 10 {
		for i := range f.fields {
			f.fields[i].textInput.Width = inputWidth
		}
	}

	return f
}
