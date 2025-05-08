// =============================================
// File: internal/task/manager.go
// =============================================
package task

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// Manager loads and parses Task definitions from CSV.
type Manager struct {
	logger *zap.Logger
}

// NewManager constructs a Manager with the given logger.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{logger: logger}
}

// LoadTasks reads tasks from CSV at path. Returns parsed Task slice.
func (m *Manager) LoadTasks(path string) ([]*Task, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open tasks file: %w", err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	// Read header
	if _, err := r.Read(); err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	tasks := make([]*Task, 0)
	line := 1
	for {
		line++
		rw, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			m.logger.Warn("Skipping malformed CSV row", zap.Int("line", line), zap.Error(err))
			continue
		}

		task, err := m.parseRow(rw, line)
		if err != nil {
			m.logger.Warn("Skipping invalid task row", zap.Int("line", line), zap.Error(err))
			continue
		}
		tasks = append(tasks, task)
	}

	m.logger.Info("Loaded tasks", zap.Int("count", len(tasks)))
	return tasks, nil
}

func (m *Manager) parseRow(fields []string, line int) (*Task, error) {
	if len(fields) < 8 {
		return nil, fmt.Errorf("expected >=8 columns, got %d", len(fields))
	}

	op, err := parseOperation(fields[3])
	if err != nil {
		return nil, err
	}

	amount, err := parseFloatField(fields[4], "amount_sol")
	if err != nil {
		return nil, err
	}

	slippage, err := parseFloatField(fields[5], "slippage_percent")
	if err != nil {
		return nil, err
	}
	slippage = clamp(slippage, 0.5, 100.0, 1.0)

	priority := fields[6]
	if priority == "" {
		priority = "default"
	}

	computeUnits, err := parseUint32Field(fields, 8)
	if err != nil {
		m.logger.Warn("Invalid compute_units, using default", zap.Error(err))
	}

	autoSell := 99.0
	if len(fields) > 9 {
		s, err := strconv.ParseFloat(fields[9], 64)
		if err == nil && s >= 1 && s <= 99 {
			autoSell = s
		} else {
			m.logger.Warn("Invalid percent_to_sell, defaulting to 99%", zap.String("value", fields[9]), zap.Error(err))
		}
	}

	return &Task{
		ID:              line - 1,
		TaskName:        fields[0],
		Module:          fields[1],
		WalletName:      fields[2],
		Operation:       op,
		AmountSol:       amount,
		SlippagePercent: slippage,
		PriorityFeeSol:  priority,
		ComputeUnits:    computeUnits,
		AutosellAmount:  autoSell,
		TokenMint:       fields[7],
		CreatedAt:       time.Now(),
	}, nil
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

func parseFloatField(value, name string) (float64, error) {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return f, nil
}

func parseUint32Field(fields []string, idx int) (uint32, error) {
	if idx >= len(fields) || fields[idx] == "" {
		return 0, nil
	}
	u, err := strconv.ParseUint(fields[idx], 10, 32)
	return uint32(u), err
}

func clamp(val, min, max, def float64) float64 {
	if val < min || val > max {
		return def
	}
	return val
}
