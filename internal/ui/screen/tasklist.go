package screen

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
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

// TaskListScreen represents the task list and execution screen
type TaskListScreen struct {
	width  int
	height int
	keyMap ui.KeyMap

	// UI components
	helpBar *component.HelpBar
	table   *component.Table

	// State
	tasks         []task.Task
	selectedTasks map[int]bool // Task ID -> selected
	executingTask int          // Currently executing task ID (-1 if none)
	errors        []string
	lastRefresh   time.Time

	// Styling
	titleStyle     lipgloss.Style
	statusStyle    lipgloss.Style
	errorStyle     lipgloss.Style
	successStyle   lipgloss.Style
	executingStyle lipgloss.Style
	infoStyle      lipgloss.Style
}

// NewTaskListScreen creates a new task list screen
func NewTaskListScreen() *TaskListScreen {
	palette := style.DefaultPalette()
	keyMap := ui.DefaultKeyMap()

	screen := &TaskListScreen{
		keyMap:        keyMap,
		selectedTasks: make(map[int]bool),
		executingTask: -1,
		errors:        make([]string, 0),
		lastRefresh:   time.Now(),

		titleStyle: lipgloss.NewStyle().
			Foreground(palette.Primary).
			Bold(true).
			Margin(1, 0).
			Align(lipgloss.Center),

		statusStyle: lipgloss.NewStyle().
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

		executingStyle: lipgloss.NewStyle().
			Foreground(palette.Warning).
			Bold(true).
			Padding(0, 2),

		infoStyle: lipgloss.NewStyle().
			Foreground(palette.Text).
			Padding(0, 2),
	}

	screen.initializeTable()
	screen.initializeHelpBar()

	return screen
}

// initializeTable sets up the tasks table
func (s *TaskListScreen) initializeTable() {
	s.table = component.NewTable().
		AddColumn("ID", 4, lipgloss.Right).
		AddColumn("Name", 15, lipgloss.Left).
		AddColumn("Module", 10, lipgloss.Left).
		AddColumn("Operation", 10, lipgloss.Left).
		AddColumn("Amount", 12, lipgloss.Right).
		AddColumn("Token", 20, lipgloss.Left).
		AddColumn("Status", 12, lipgloss.Center).
		SetShowBorder(true).
		SetSelectable(true).
		SetZebra(true)
}

// initializeHelpBar sets up the help bar
func (s *TaskListScreen) initializeHelpBar() {
	s.helpBar = component.NewHelpBar().
		SetKeyBindings(s.keyMap.ContextualHelp(ui.RouteTaskList)).
		SetCompact(false)
}

// Init initializes the task list screen
func (s *TaskListScreen) Init() tea.Cmd {
	return tea.Batch(
		ui.ListenBus(),
		s.loadTasksCmd(),
		tea.Tick(time.Second*30, func(t time.Time) tea.Msg {
			return RefreshTasksMsg{Time: t}
		}),
	)
}

// RefreshTasksMsg is sent periodically to refresh the task list
type RefreshTasksMsg struct {
	Time time.Time
}

// TasksLoadedMsg is sent when tasks are loaded from CSV
type TasksLoadedMsg struct {
	Tasks []task.Task
	Error error
}

// TaskExecutionMsg is sent when task execution starts/stops
type TaskExecutionMsg struct {
	TaskID    int
	Status    string
	Error     error
	Completed bool
}

// Update handles screen updates
func (s *TaskListScreen) Update(msg tea.Msg) (router.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, s.keyMap.Quit):
			return s, tea.Quit

		case key.Matches(msg, s.keyMap.Back):
			// Go back to main menu
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteMainMenu}
			})

		case key.Matches(msg, s.keyMap.Up):
			s.table.MoveUp()

		case key.Matches(msg, s.keyMap.Down):
			s.table.MoveDown()

		case key.Matches(msg, s.keyMap.Enter):
			// Execute selected task
			if s.executingTask == -1 {
				selectedRow := s.table.GetSelectedRow()
				if selectedRow < len(s.tasks) {
					taskToExecute := s.tasks[selectedRow]
					s.executingTask = taskToExecute.ID
					cmds = append(cmds, s.executeTaskCmd(taskToExecute))
				}
			}

		case key.Matches(msg, s.keyMap.Refresh):
			// Refresh task list
			cmds = append(cmds, s.loadTasksCmd())

		case key.Matches(msg, s.keyMap.CreateTask):
			// Navigate to create task screen
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteCreateTask}
			})

		case key.Matches(msg, s.keyMap.Monitor):
			// Navigate to monitor screen
			cmds = append(cmds, func() tea.Msg {
				return ui.RouterMsg{To: ui.RouteMonitor}
			})

		case msg.String() == " ":
			// Toggle task selection for batch operations
			selectedRow := s.table.GetSelectedRow()
			if selectedRow < len(s.tasks) {
				taskID := s.tasks[selectedRow].ID
				s.selectedTasks[taskID] = !s.selectedTasks[taskID]
				s.updateTableDisplay()
			}

		case msg.String() == "a":
			// Select all tasks
			for _, task := range s.tasks {
				s.selectedTasks[task.ID] = true
			}
			s.updateTableDisplay()

		case msg.String() == "d":
			// Deselect all tasks
			s.selectedTasks = make(map[int]bool)
			s.updateTableDisplay()

		case msg.String() == "r":
			// Run selected tasks
			if s.executingTask == -1 {
				cmds = append(cmds, s.executeSelectedTasksCmd())
			}
		}

	case RefreshTasksMsg:
		// Auto-refresh tasks
		s.lastRefresh = msg.Time
		cmds = append(cmds, s.loadTasksCmd())
		// Schedule next refresh
		cmds = append(cmds, tea.Tick(time.Second*30, func(t time.Time) tea.Msg {
			return RefreshTasksMsg{Time: t}
		}))

	case TasksLoadedMsg:
		if msg.Error != nil {
			s.errors = []string{fmt.Sprintf("Error loading tasks: %v", msg.Error)}
		} else {
			s.tasks = msg.Tasks
			s.errors = make([]string, 0)
			s.updateTableDisplay()
		}

	case TaskExecutionMsg:
		if msg.Completed {
			s.executingTask = -1
			if msg.Error != nil {
				s.errors = append(s.errors, fmt.Sprintf("Task execution failed: %v", msg.Error))
			}
		}

	case ui.DomainEventMsg:
		// Handle domain events (task completion, etc.)
		s.handleDomainEvent(msg.Event)

	case ui.ErrorMsg:
		s.errors = append(s.errors, msg.Error.Error())

	case ui.SuccessMsg:
		// Clear errors on success
		s.errors = make([]string, 0)
	}

	// Continue listening for events
	cmds = append(cmds, ui.ListenBus())

	return s, tea.Batch(cmds...)
}

// View renders the task list screen
func (s *TaskListScreen) View() string {
	if s.width == 0 || s.height == 0 {
		return "Loading..."
	}

	var content strings.Builder

	// Title
	title := "📋 Trading Tasks"
	content.WriteString(s.titleStyle.Width(s.width).Render(title))
	content.WriteString("\n\n")

	// Status bar
	statusBar := s.renderStatusBar()
	content.WriteString(statusBar)
	content.WriteString("\n\n")

	// Error messages
	if len(s.errors) > 0 {
		for _, err := range s.errors {
			content.WriteString(s.errorStyle.Render("❌ " + err))
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	// Tasks table
	if len(s.tasks) > 0 {
		content.WriteString(s.table.View())
	} else {
		emptyMsg := "No tasks found. Press 'c' to create your first task."
		content.WriteString(s.infoStyle.Render(emptyMsg))
	}

	content.WriteString("\n")

	// Instructions
	instructions := s.renderInstructions()
	content.WriteString(instructions)
	content.WriteString("\n")

	// Help bar
	help := s.helpBar.SetWidth(s.width).View()
	content.WriteString(help)

	return content.String()
}

// SetSize sets the screen dimensions
func (s *TaskListScreen) SetSize(width, height int) {
	s.width = width
	s.height = height
	s.helpBar.SetWidth(width)
	s.table.SetSize(width-4, height-15) // Leave space for other elements
}

// renderStatusBar renders the status information
func (s *TaskListScreen) renderStatusBar() string {
	var statusParts []string

	// Task count
	statusParts = append(statusParts, fmt.Sprintf("Tasks: %d", len(s.tasks)))

	// Selected count
	selectedCount := len(s.selectedTasks)
	if selectedCount > 0 {
		statusParts = append(statusParts, fmt.Sprintf("Selected: %d", selectedCount))
	}

	// Execution status
	if s.executingTask != -1 {
		statusParts = append(statusParts, "⚡ Executing...")
	}

	// Last refresh
	statusParts = append(statusParts, fmt.Sprintf("Updated: %s", s.lastRefresh.Format("15:04:05")))

	statusLine := strings.Join(statusParts, " • ")
	return s.statusStyle.Render(statusLine)
}

// renderInstructions renders usage instructions
func (s *TaskListScreen) renderInstructions() string {
	var instructions []string

	if s.executingTask == -1 {
		instructions = append(instructions, "Enter: Run task")
		instructions = append(instructions, "Space: Toggle selection")
		instructions = append(instructions, "R: Run selected")
		instructions = append(instructions, "A: Select all")
		instructions = append(instructions, "D: Deselect all")
	} else {
		instructions = append(instructions, "Task is executing, please wait...")
	}

	instructions = append(instructions, "C: Create task")
	instructions = append(instructions, "M: Monitor")
	instructions = append(instructions, "F5: Refresh")

	return s.infoStyle.Render(strings.Join(instructions, " • "))
}

// updateTableDisplay updates the table with current task data
func (s *TaskListScreen) updateTableDisplay() {
	if s.table == nil {
		return
	}

	rows := make([][]string, 0, len(s.tasks))
	palette := style.DefaultPalette()

	for i, task := range s.tasks {
		// Format amount
		amountStr := fmt.Sprintf("%.6f SOL", task.AmountSol)

		// Truncate token mint for display
		tokenDisplay := task.TokenMint
		if len(tokenDisplay) > 18 {
			tokenDisplay = tokenDisplay[:15] + "..."
		}

		// Determine status
		status := "Ready"
		if task.ID == s.executingTask {
			status = "⚡ Running"
		} else if s.selectedTasks[task.ID] {
			status = "✓ Selected"
		}

		row := []string{
			fmt.Sprintf("%d", task.ID),
			task.TaskName,
			task.Module,
			string(task.Operation),
			amountStr,
			tokenDisplay,
			status,
		}

		rows = append(rows, row)

		// Set custom styling for special rows
		if task.ID == s.executingTask {
			// Executing task - highlight
			style := lipgloss.NewStyle().
				Foreground(palette.Background).
				Background(palette.Warning).
				Bold(true)
			s.table.SetRowStyle(i, style)
		} else if s.selectedTasks[task.ID] {
			// Selected task - subtle highlight
			style := lipgloss.NewStyle().
				Foreground(palette.Primary).
				Bold(true)
			s.table.SetRowStyle(i, style)
		}
	}

	s.table.SetRows(rows)
}

// loadTasksCmd creates a command to load tasks from CSV
func (s *TaskListScreen) loadTasksCmd() tea.Cmd {
	return func() tea.Msg {
		tasks, err := s.loadTasksFromCSV()
		return TasksLoadedMsg{
			Tasks: tasks,
			Error: err,
		}
	}
}

// loadTasksFromCSV loads tasks from the CSV file
func (s *TaskListScreen) loadTasksFromCSV() ([]task.Task, error) {
	csvPath := filepath.Join("configs", "tasks.csv")

	// Validate file path to prevent directory traversal
	if !strings.HasPrefix(csvPath, "configs/") {
		return nil, fmt.Errorf("invalid file path")
	}

	file, err := os.Open(filepath.Clean(csvPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open tasks.csv: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log close error but don't override the main error
			fmt.Printf("Warning: failed to close CSV file: %v\n", closeErr)
		}
	}()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) < 2 {
		return []task.Task{}, nil // Empty file or only headers
	}

	tasks := make([]task.Task, 0, len(records)-1)

	// Skip header row
	for i, record := range records[1:] {
		if len(record) < 10 {
			continue // Skip incomplete rows
		}

		// Parse the CSV record into a task
		taskItem := task.Task{
			ID:         i + 1, // Use row index as ID
			TaskName:   record[0],
			Module:     record[1],
			WalletName: record[2],
			Operation:  task.OperationType(record[3]),
			CreatedAt:  time.Now(),
		}

		// Parse numeric fields
		if amount, err := strconv.ParseFloat(record[4], 64); err == nil {
			taskItem.AmountSol = amount
		}

		if slippage, err := strconv.ParseFloat(record[5], 64); err == nil {
			taskItem.SlippagePercent = slippage
		}

		taskItem.PriorityFeeSol = record[6]
		taskItem.TokenMint = record[7]

		if compute, err := strconv.ParseUint(record[8], 10, 32); err == nil {
			taskItem.ComputeUnits = uint32(compute)
		}

		if autosell, err := strconv.ParseFloat(record[9], 64); err == nil {
			taskItem.AutosellAmount = autosell
		}

		tasks = append(tasks, taskItem)
	}

	return tasks, nil
}

// executeTaskCmd creates a command to execute a single task
func (s *TaskListScreen) executeTaskCmd(task task.Task) tea.Cmd {
	return func() tea.Msg {
		// Create domain event for task execution
		event := domain.Event{
			Type:      domain.EventTaskExecuted,
			Timestamp: time.Now(),
			Data:      task,
		}

		// Send event through the bus
		ui.Bus <- ui.DomainEventMsg{Event: event}

		// TODO: Actually execute the task
		// This would involve calling the trading bot services
		// For now, simulate execution
		time.Sleep(time.Second * 2) // Simulate processing time

		return TaskExecutionMsg{
			TaskID:    task.ID,
			Status:    "completed",
			Completed: true,
			Error:     nil,
		}
	}
}

// executeSelectedTasksCmd creates a command to execute selected tasks
func (s *TaskListScreen) executeSelectedTasksCmd() tea.Cmd {
	var selectedTasks []task.Task

	for _, task := range s.tasks {
		if s.selectedTasks[task.ID] {
			selectedTasks = append(selectedTasks, task)
		}
	}

	if len(selectedTasks) == 0 {
		return func() tea.Msg {
			return ui.ErrorMsg{Error: fmt.Errorf("no tasks selected")}
		}
	}

	// For now, just execute the first selected task
	// TODO: Implement batch execution
	return s.executeTaskCmd(selectedTasks[0])
}

// handleDomainEvent processes domain events
func (s *TaskListScreen) handleDomainEvent(event domain.Event) {
	switch event.Type {
	case domain.EventTaskCreated:
		// Refresh tasks when a new task is created
		s.loadTasksCmd()

	case domain.EventTaskExecuted:
		// Task execution started/completed

	case domain.EventPriceTick:
		// Price updates during monitoring

	default:
		// Handle other events as needed
	}
}

// GetSelectedTasks returns the currently selected tasks
func (s *TaskListScreen) GetSelectedTasks() []task.Task {
	var selected []task.Task
	for _, task := range s.tasks {
		if s.selectedTasks[task.ID] {
			selected = append(selected, task)
		}
	}
	return selected
}

// GetSelectedTaskCount returns the number of selected tasks
func (s *TaskListScreen) GetSelectedTaskCount() int {
	count := 0
	for _, selected := range s.selectedTasks {
		if selected {
			count++
		}
	}
	return count
}

// IsExecuting returns true if a task is currently executing
func (s *TaskListScreen) IsExecuting() bool {
	return s.executingTask != -1
}
