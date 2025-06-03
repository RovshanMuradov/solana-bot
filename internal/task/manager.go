package task

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Manager loads and parses Task definitions.
type Manager struct {
	logger *zap.Logger
}

// TaskConfig represents the structure of tasks YAML file
type TaskConfig struct {
	Tasks []struct {
		TaskName        string  `yaml:"task_name"`
		Module          string  `yaml:"module"`
		Wallet          string  `yaml:"wallet"`
		Operation       string  `yaml:"operation"`
		AmountSol       float64 `yaml:"amount_sol"`
		SlippagePercent float64 `yaml:"slippage_percent"`
		PriorityFee     string  `yaml:"priority_fee"`
		ComputeUnits    uint32  `yaml:"compute_units"`
		PercentToSell   float64 `yaml:"percent_to_sell"`
		TokenMint       string  `yaml:"token_mint"`
	} `yaml:"tasks"`
}

// NewManager constructs a Manager with the given logger.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{logger: logger}
}

func parseOperation(s string) (OperationType, error) {
	op := OperationType(s)
	switch op {
	case OperationSnipe, OperationSwap, OperationSell:
		return op, nil
	default:
		return "", fmt.Errorf("unsupported operation: %q", s)
	}
}

func clamp(val, min, max, def float64) float64 {
	if val < min || val > max {
		return def
	}
	return val
}

// LoadTasksYAML reads tasks from YAML file
func (m *Manager) LoadTasksYAML(path string) ([]*Task, error) {
	// Validate file path to prevent path traversal
	if filepath.IsAbs(path) {
		m.logger.Warn("Using absolute path for tasks file", zap.String("path", path))
	}

	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config TaskConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if len(config.Tasks) == 0 {
		return nil, fmt.Errorf("no tasks found in configuration")
	}

	tasks := make([]*Task, 0, len(config.Tasks))
	for i, taskData := range config.Tasks {
		// Validate operation
		op, err := parseOperation(taskData.Operation)
		if err != nil {
			m.logger.Warn("Skipping invalid task", zap.String("task_name", taskData.TaskName), zap.Error(err))
			continue
		}

		// Validate and clamp values
		slippage := clamp(taskData.SlippagePercent, 0.5, 100.0, 1.0)

		priority := taskData.PriorityFee
		if priority == "" {
			priority = "default"
		}

		autoSell := taskData.PercentToSell
		if autoSell <= 0 || autoSell > 100 {
			autoSell = 99.0
		}

		task := &Task{
			ID:              i,
			TaskName:        taskData.TaskName,
			Module:          taskData.Module,
			WalletName:      taskData.Wallet,
			Operation:       op,
			AmountSol:       taskData.AmountSol,
			SlippagePercent: slippage,
			PriorityFeeSol:  priority,
			ComputeUnits:    taskData.ComputeUnits,
			AutosellAmount:  autoSell,
			TokenMint:       taskData.TokenMint,
			CreatedAt:       time.Now(),
		}

		// Validate required fields
		if task.TaskName == "" || task.Module == "" || task.WalletName == "" || task.TokenMint == "" {
			m.logger.Warn("Skipping task with missing required fields",
				zap.String("task_name", task.TaskName),
				zap.String("module", task.Module),
				zap.String("wallet", task.WalletName),
				zap.String("token_mint", task.TokenMint))
			continue
		}

		// Validate amount for buy operations
		if (task.Operation == OperationSnipe || task.Operation == OperationSwap) && task.AmountSol <= 0 {
			m.logger.Warn("Skipping buy operation with invalid amount",
				zap.String("task_name", task.TaskName),
				zap.Float64("amount", task.AmountSol))
			continue
		}

		tasks = append(tasks, task)
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("no valid tasks loaded")
	}

	m.logger.Info("Loaded tasks", zap.Int("count", len(tasks)))
	return tasks, nil
}

// LoadTasks is a wrapper that delegates to LoadTasksYAML
func (m *Manager) LoadTasks(path string) ([]*Task, error) {
	return m.LoadTasksYAML(path)
}
