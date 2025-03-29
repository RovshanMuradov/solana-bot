// =============================================
// File: internal/task/models.go
// =============================================
package task

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"go.uber.org/zap"
)

// Manager handles task loading and processing
type Manager struct {
	logger *zap.Logger
}

// NewManager creates a new task manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{logger: logger}
}

// LoadTasks reads tasks from CSV file
// CSV format: task_name,module,wallet,operation,amount,slippage_percent,priority_fee_sol,token_mint,compute_units
func (m *Manager) LoadTasks(path string) ([]*Task, error) {
	m.logger.Debug("Loading tasks", zap.String("path", path))

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file error: %w", err)
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read CSV error: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("no tasks found (need header + at least one task)")
	}

	// Process records (skip header)
	tasks := make([]*Task, 0, len(records)-1)

	for i, row := range records[1:] {
		if len(row) < 8 {
			m.logger.Warn("Skipping row with insufficient columns",
				zap.Int("row", i+2),
				zap.Int("columns", len(row)))
			continue
		}

		task, err := m.parseTaskRow(row, i)
		if err != nil {
			m.logger.Warn("Skipping invalid task row",
				zap.Int("row", i+2),
				zap.Error(err))
			continue
		}

		tasks = append(tasks, task)
	}

	m.logger.Info("Tasks loaded successfully", zap.Int("count", len(tasks)))
	return tasks, nil
}

// parseTaskRow parses a single row from the CSV into a Task object
func (m *Manager) parseTaskRow(row []string, index int) (*Task, error) {
	// Parse operation
	op := OperationType(row[3])
	switch op {
	case OperationSnipe, OperationSell, OperationSwap:
		// Valid operations
	default:
		return nil, fmt.Errorf("invalid operation: %s", op)
	}

	// Parse amount
	amountSol, err := strconv.ParseFloat(row[4], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid amount_sol value: %w", err)
	}

	// Parse slippage
	slippagePercent, err := strconv.ParseFloat(row[5], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid slippage_percent value: %w", err)
	}

	// Validate slippage
	const MinSlippage = 0.5
	if slippagePercent < MinSlippage || slippagePercent > 100 {
		m.logger.Warn("Slippage percent out of range, using default",
			zap.Float64("provided", slippagePercent),
			zap.Float64("min_allowed", MinSlippage))
		slippagePercent = 1.0
	}

	// Parse priority fee (use default if empty)
	priorityFeeSol := row[6]
	if priorityFeeSol == "" {
		priorityFeeSol = "default"
	}

	// Parse compute units (default is 0 which means use default value)
	var computeUnits uint32
	if len(row) > 8 && row[8] != "" {
		computeUnitsUint64, err := strconv.ParseUint(row[8], 10, 32)
		if err != nil {
			m.logger.Warn("Invalid compute_units value, using default",
				zap.String("value", row[8]),
				zap.Error(err))
		} else {
			computeUnits = uint32(computeUnitsUint64)
		}
	}

	return &Task{
		ID:              index + 1,
		TaskName:        row[0],
		Module:          row[1],
		WalletName:      row[2],
		Operation:       op,
		AmountSol:       amountSol,
		SlippagePercent: slippagePercent,
		PriorityFeeSol:  priorityFeeSol,
		ComputeUnits:    computeUnits,
		TokenMint:       row[7],
	}, nil
}
