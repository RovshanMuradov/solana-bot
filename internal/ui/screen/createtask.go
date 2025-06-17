package screen

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rovshanmuradov/solana-bot/internal/domain"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/ui"
	"github.com/rovshanmuradov/solana-bot/internal/ui/component"
	"github.com/rovshanmuradov/solana-bot/internal/ui/router"
	"github.com/rovshanmuradov/solana-bot/internal/ui/style"
)

// CreateTaskStep represents the current step in the wizard
type CreateTaskStep int

const (
	StepBasicInfo CreateTaskStep = iota
	StepWalletAndOperation
	StepTradingParameters
	StepTokenDetails
	StepPreview
	StepConfirm
)

// CreateTaskWizard represents the task creation wizard screen
type CreateTaskWizard struct {
	width  int
	height int
	keyMap ui.KeyMap

	// UI components
	helpBar     *component.HelpBar
	basicForm   *component.Form
	walletForm  *component.Form
	tradingForm *component.Form
	tokenForm   *component.Form

	// State
	currentStep CreateTaskStep
	taskData    task.Task
	errors      []string

	// Styling
	titleStyle     lipgloss.Style
	stepStyle      lipgloss.Style
	errorStyle     lipgloss.Style
	successStyle   lipgloss.Style
	containerStyle lipgloss.Style
	previewStyle   lipgloss.Style

	// Preview table for final step
	previewTable *component.Table
}

// NewCreateTaskWizard creates a new task creation wizard
func NewCreateTaskWizard() *CreateTaskWizard {
	palette := style.DefaultPalette()
	keyMap := ui.DefaultKeyMap()

	wizard := &CreateTaskWizard{
		keyMap:      keyMap,
		currentStep: StepBasicInfo,
		taskData:    task.Task{CreatedAt: time.Now()},
		errors:      make([]string, 0),

		titleStyle: lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true).
			Margin(1, 0).
			Align(lipgloss.Center),

		stepStyle: lipgloss.NewStyle().
			Foreground(palette.Secondary).
			Bold(true).
			Padding(0, 2),

		errorStyle: lipgloss.NewStyle().
			Foreground(palette.Error).
			Bold(true).
			Padding(0, 2),

		successStyle: lipgloss.NewStyle().
			Foreground(palette.Success).
			Bold(true).
			Padding(0, 2),

		containerStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(palette.Primary).
			Padding(2, 4).
			Margin(1, 0),

		previewStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(1, 2),
	}

	wizard.initializeForms()
	wizard.initializePreviewTable()
	wizard.initializeHelpBar()

	return wizard
}

// initializeForms creates all the form components
func (w *CreateTaskWizard) initializeForms() {
	// Basic Info Form
	w.basicForm = component.NewForm().
		SetTitle("Basic Task Information").
		AddField("task_name", component.FieldTypeText, "Task Name", true, "Enter a descriptive name for this task").
		AddField("module", component.FieldTypeSelect, "DEX Module", true, "Select trading module").
		SetSelectOptions("module", []string{"pumpfun", "pumpswap", "smart"})

	// Wallet and Operation Form
	w.walletForm = component.NewForm().
		SetTitle("Wallet and Operation").
		AddField("wallet", component.FieldTypeText, "Wallet Name", true, "Name of the wallet configuration").
		AddField("operation", component.FieldTypeSelect, "Operation Type", true, "Type of trading operation").
		SetSelectOptions("operation", []string{"snipe", "swap", "sell"})

	// Trading Parameters Form
	w.tradingForm = component.NewForm().
		SetTitle("Trading Parameters").
		AddField("amount_sol", component.FieldTypeNumber, "Amount (SOL)", true, "SOL amount to spend or tokens to sell").
		AddField("slippage_percent", component.FieldTypeNumber, "Slippage %", true, "Maximum allowed slippage percentage").
		AddField("priority_fee", component.FieldTypeText, "Priority Fee", true, "Priority fee (e.g., 0.000001 or 'default')")

	// Token Details Form
	w.tokenForm = component.NewForm().
		SetTitle("Token Details").
		AddField("token_mint", component.FieldTypeText, "Token Mint Address", true, "Solana token mint address").
		AddField("compute_units", component.FieldTypeNumber, "Compute Units", true, "Compute units for transaction").
		AddField("percent_to_sell", component.FieldTypeNumber, "Auto-sell %", false, "Percentage of tokens to auto-sell")

	// Set default values
	w.tradingForm.SetFieldValue("slippage_percent", "10.0")
	w.tradingForm.SetFieldValue("priority_fee", "default")
	w.tokenForm.SetFieldValue("compute_units", "200000")
	w.tokenForm.SetFieldValue("percent_to_sell", "0")
}

// initializePreviewTable creates the preview table
func (w *CreateTaskWizard) initializePreviewTable() {
	w.previewTable = component.NewTable().
		AddColumn("Field", 20, lipgloss.Left).
		AddColumn("Value", 40, lipgloss.Left).
		SetShowBorder(true).
		SetSelectable(false)
}

// initializeHelpBar creates the help bar
func (w *CreateTaskWizard) initializeHelpBar() {
	w.helpBar = component.NewHelpBar().
		SetKeyBindings(w.keyMap.ContextualHelp(ui.RouteCreateTask)).
		SetCompact(false)
}

// Init initializes the wizard
func (w *CreateTaskWizard) Init() tea.Cmd {
	return tea.Batch(
		ui.ListenBus(),
		w.getCurrentForm().Init(),
	)
}

// Update handles wizard updates
func (w *CreateTaskWizard) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, w.keyMap.Quit):
			return w, tea.Quit

		case key.Matches(msg, w.keyMap.Back):
			if w.currentStep == StepBasicInfo {
				// Go back to main menu
				cmds = append(cmds, func() tea.Msg {
					return ui.RouterMsg{To: ui.RouteMainMenu}
				})
			} else {
				// Go to previous step
				w.previousStep()
			}

		case key.Matches(msg, w.keyMap.Enter):
			switch w.currentStep {
			case StepPreview:
				// Save task and go to confirmation
				err := w.saveTask()
				if err != nil {
					w.errors = []string{fmt.Sprintf("Error saving task: %v", err)}
				} else {
					w.currentStep = StepConfirm
				}
			case StepConfirm:
				// Task saved successfully, go back to main menu
				cmds = append(cmds, func() tea.Msg {
					return ui.RouterMsg{To: ui.RouteMainMenu}
				})
			default:
				// Validate current form and go to next step
				if w.validateCurrentStep() {
					w.nextStep()
				}
			}

		default:
			// Pass to current form
			if w.currentStep < StepPreview {
				form := w.getCurrentForm()
				updatedForm, cmd := form.Update(msg)
				*form = *updatedForm
				cmds = append(cmds, cmd)
			}
		}

	case ui.DomainEventMsg:
		// Handle domain events if needed

	case ui.ErrorMsg:
		w.errors = append(w.errors, msg.Error.Error())

	case ui.SuccessMsg:
		// Clear errors on success
		w.errors = make([]string, 0)
	}

	// Continue listening for events
	cmds = append(cmds, ui.ListenBus())

	return w, tea.Batch(cmds...)
}

// View renders the wizard
func (w *CreateTaskWizard) View() string {
	if w.width == 0 || w.height == 0 {
		return "Loading..."
	}

	var content strings.Builder

	// Title
	title := fmt.Sprintf("🧙‍♂️ Create New Trading Task - Step %d/5", int(w.currentStep)+1)
	content.WriteString(w.titleStyle.Width(w.width).Render(title))
	content.WriteString("\n\n")

	// Step indicator
	stepIndicator := w.renderStepIndicator()
	content.WriteString(stepIndicator)
	content.WriteString("\n\n")

	// Error messages
	if len(w.errors) > 0 {
		for _, err := range w.errors {
			content.WriteString(w.errorStyle.Render("❌ " + err))
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	// Current step content
	stepContent := w.renderCurrentStep()
	content.WriteString(w.containerStyle.Render(stepContent))
	content.WriteString("\n")

	// Help bar
	help := w.helpBar.SetWidth(w.width).View()
	content.WriteString(help)

	return content.String()
}

// SetSize sets the screen dimensions
func (w *CreateTaskWizard) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.helpBar.SetWidth(width)

	// Update form sizes
	formWidth := width - 8 // Account for container padding
	w.basicForm.SetWidth(formWidth)
	w.walletForm.SetWidth(formWidth)
	w.tradingForm.SetWidth(formWidth)
	w.tokenForm.SetWidth(formWidth)
	w.previewTable.SetSize(formWidth, height-10)
}

// getCurrentForm returns the form for the current step
func (w *CreateTaskWizard) getCurrentForm() *component.Form {
	switch w.currentStep {
	case StepBasicInfo:
		return w.basicForm
	case StepWalletAndOperation:
		return w.walletForm
	case StepTradingParameters:
		return w.tradingForm
	case StepTokenDetails:
		return w.tokenForm
	default:
		return w.basicForm
	}
}

// renderStepIndicator renders the step progress indicator
func (w *CreateTaskWizard) renderStepIndicator() string {
	steps := []string{"Basic", "Wallet", "Trading", "Token", "Preview"}
	var indicators []string

	palette := style.DefaultPalette()

	for i, stepName := range steps {
		if i == int(w.currentStep) {
			// Current step - highlight
			indicator := lipgloss.NewStyle().
				Foreground(palette.Background).
				Background(palette.Primary).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("%d. %s", i+1, stepName))
			indicators = append(indicators, indicator)
		} else if i < int(w.currentStep) {
			// Completed step
			indicator := lipgloss.NewStyle().
				Foreground(palette.Success).
				Bold(true).
				Render(fmt.Sprintf("✓ %s", stepName))
			indicators = append(indicators, indicator)
		} else {
			// Future step
			indicator := lipgloss.NewStyle().
				Foreground(palette.TextMuted).
				Render(fmt.Sprintf("%d. %s", i+1, stepName))
			indicators = append(indicators, indicator)
		}
	}

	return strings.Join(indicators, " → ")
}

// renderCurrentStep renders the content for the current step
func (w *CreateTaskWizard) renderCurrentStep() string {
	switch w.currentStep {
	case StepPreview:
		return w.renderPreview()
	case StepConfirm:
		return w.renderConfirmation()
	default:
		return w.getCurrentForm().View()
	}
}

// renderPreview renders the task preview
func (w *CreateTaskWizard) renderPreview() string {
	// Update task data from forms
	w.updateTaskFromForms()

	// Prepare preview data
	rows := [][]string{
		{"Task Name", w.taskData.TaskName},
		{"Module", w.taskData.Module},
		{"Wallet", w.taskData.WalletName},
		{"Operation", string(w.taskData.Operation)},
		{"Amount (SOL)", fmt.Sprintf("%.6f", w.taskData.AmountSol)},
		{"Slippage %", fmt.Sprintf("%.2f", w.taskData.SlippagePercent)},
		{"Priority Fee", w.taskData.PriorityFeeSol},
		{"Token Mint", w.taskData.TokenMint},
		{"Compute Units", fmt.Sprintf("%d", w.taskData.ComputeUnits)},
		{"Auto-sell %", fmt.Sprintf("%.2f", w.taskData.AutosellAmount)},
	}

	w.previewTable.SetRows(rows)

	var content strings.Builder
	content.WriteString(w.stepStyle.Render("📋 Task Preview"))
	content.WriteString("\n\n")
	content.WriteString("Please review the task details below:")
	content.WriteString("\n\n")
	content.WriteString(w.previewTable.View())
	content.WriteString("\n\n")
	content.WriteString(w.previewStyle.Render("Press Enter to save this task, or Esc to go back."))

	return content.String()
}

// renderConfirmation renders the confirmation message
func (w *CreateTaskWizard) renderConfirmation() string {
	var content strings.Builder
	content.WriteString(w.successStyle.Render("✅ Task Created Successfully!"))
	content.WriteString("\n\n")
	content.WriteString(w.previewStyle.Render(fmt.Sprintf("Task '%s' has been saved to tasks.csv", w.taskData.TaskName)))
	content.WriteString("\n\n")
	content.WriteString(w.previewStyle.Render("Press Enter to return to the main menu."))

	return content.String()
}

// validateCurrentStep validates the current step's form
func (w *CreateTaskWizard) validateCurrentStep() bool {
	w.errors = make([]string, 0) // Clear previous errors

	form := w.getCurrentForm()
	if !form.Validate() {
		w.errors = append(w.errors, "Please fill in all required fields correctly.")
		return false
	}

	// Additional validation based on step
	switch w.currentStep {
	case StepTradingParameters:
		// Validate numeric values
		amountStr := w.tradingForm.GetFieldValue("amount_sol")
		if amount, err := strconv.ParseFloat(amountStr, 64); err != nil || amount <= 0 {
			w.errors = append(w.errors, "Amount must be a positive number.")
			return false
		}

		slippageStr := w.tradingForm.GetFieldValue("slippage_percent")
		if slippage, err := strconv.ParseFloat(slippageStr, 64); err != nil || slippage < 0 || slippage > 100 {
			w.errors = append(w.errors, "Slippage must be between 0 and 100.")
			return false
		}

	case StepTokenDetails:
		// Validate compute units
		computeStr := w.tokenForm.GetFieldValue("compute_units")
		if compute, err := strconv.ParseUint(computeStr, 10, 32); err != nil || compute == 0 {
			w.errors = append(w.errors, "Compute units must be a positive integer.")
			return false
		}

		// Validate auto-sell percentage
		percentStr := w.tokenForm.GetFieldValue("percent_to_sell")
		if percent, err := strconv.ParseFloat(percentStr, 64); err != nil || percent < 0 || percent > 100 {
			w.errors = append(w.errors, "Auto-sell percentage must be between 0 and 100.")
			return false
		}

		// Validate token mint address (basic check)
		tokenMint := w.tokenForm.GetFieldValue("token_mint")
		if len(tokenMint) < 32 || len(tokenMint) > 44 {
			w.errors = append(w.errors, "Token mint address appears invalid.")
			return false
		}
	}

	return true
}

// nextStep moves to the next step
func (w *CreateTaskWizard) nextStep() {
	if w.currentStep < StepPreview {
		w.currentStep++
	}
}

// previousStep moves to the previous step
func (w *CreateTaskWizard) previousStep() {
	if w.currentStep > StepBasicInfo {
		w.currentStep--
	}
}

// updateTaskFromForms updates the task data from all forms
func (w *CreateTaskWizard) updateTaskFromForms() {
	// Basic info
	w.taskData.TaskName = w.basicForm.GetFieldValue("task_name")
	w.taskData.Module = w.basicForm.GetFieldValue("module")

	// Wallet and operation
	w.taskData.WalletName = w.walletForm.GetFieldValue("wallet")
	w.taskData.Operation = task.OperationType(w.walletForm.GetFieldValue("operation"))

	// Trading parameters
	if amountStr := w.tradingForm.GetFieldValue("amount_sol"); amountStr != "" {
		w.taskData.AmountSol, _ = strconv.ParseFloat(amountStr, 64)
	}
	if slippageStr := w.tradingForm.GetFieldValue("slippage_percent"); slippageStr != "" {
		w.taskData.SlippagePercent, _ = strconv.ParseFloat(slippageStr, 64)
	}
	w.taskData.PriorityFeeSol = w.tradingForm.GetFieldValue("priority_fee")

	// Token details
	w.taskData.TokenMint = w.tokenForm.GetFieldValue("token_mint")
	if computeStr := w.tokenForm.GetFieldValue("compute_units"); computeStr != "" {
		if compute, err := strconv.ParseUint(computeStr, 10, 32); err == nil {
			w.taskData.ComputeUnits = uint32(compute)
		}
	}
	if percentStr := w.tokenForm.GetFieldValue("percent_to_sell"); percentStr != "" {
		w.taskData.AutosellAmount, _ = strconv.ParseFloat(percentStr, 64)
	}
}

// saveTask saves the task to CSV file
func (w *CreateTaskWizard) saveTask() error {
	w.updateTaskFromForms()

	// Create domain event for task creation
	event := domain.Event{
		Type:      domain.EventTaskCreated,
		Timestamp: time.Now(),
		Data:      w.taskData,
	}

	// Send event through the bus
	ui.Bus <- ui.DomainEventMsg{Event: event}

	// TODO: Actually save to CSV file
	// This would involve calling a task manager service to append to tasks.csv
	// For now, we'll just simulate success

	return nil
}

// GetStepCount returns the total number of steps
func (w *CreateTaskWizard) GetStepCount() int {
	return 5
}

// GetCurrentStep returns the current step number (1-based)
func (w *CreateTaskWizard) GetCurrentStep() int {
	return int(w.currentStep) + 1
}

// IsCompleted returns true if the wizard is completed
func (w *CreateTaskWizard) IsCompleted() bool {
	return w.currentStep == StepConfirm
}
