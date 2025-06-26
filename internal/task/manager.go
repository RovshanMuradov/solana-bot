// =============================================
// File: internal/task/manager.go
// =============================================
package task

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	// Validate file path to prevent path traversal
	if filepath.IsAbs(path) {
		m.logger.Warn("⚠️  Using absolute path for tasks file: " + path)
	}

	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open tasks file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			m.logger.Error("❌ Failed to close tasks file: " + err.Error())
		}
	}()

	r := csv.NewReader(file)
	// Read header
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	indexes := make(map[string]int)
	for i, name := range header {
		indexes[name] = i
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
			m.logger.Warn(fmt.Sprintf("⚠️  Skipping malformed CSV row at line %d: %v", line, err))
			continue
		}

		task, err := m.parseRow(rw, indexes, line)
		if err != nil {
			m.logger.Warn(fmt.Sprintf("⚠️  Skipping invalid task at line %d: %v", line, err))
			continue
		}
		tasks = append(tasks, task)
	}

	m.logger.Info(fmt.Sprintf("📋 Loaded %d trading tasks", len(tasks)))
	return tasks, nil
}

func (m *Manager) parseRow(fields []string, indexes map[string]int, line int) (*Task, error) {
	get := func(key string) string {
		if idx, ok := indexes[key]; ok && idx < len(fields) {
			return fields[idx]
		}
		return ""
	}

	op, err := parseOperation(get("operation"))
	if err != nil {
		return nil, err
	}

	amount, err := parseFloatField(get("amount_sol"), "amount_sol")
	if err != nil {
		return nil, err
	}

	slippage, err := parseFloatField(get("slippage_percent"), "slippage_percent")
	if err != nil {
		return nil, err
	}
	slippage = clamp(slippage, 0.5, 100.0, 1.0)

	priority := get("priority_fee")
	if priority == "" {
		priority = "default"
	}

	computeUnits, err := parseUint32FieldStr(get("compute_units"))
	if err != nil {
		m.logger.Warn("⚠️  Invalid compute_units, using default: " + err.Error())
	}

	autoSell := 99.0
	if s := get("percent_to_sell"); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 1 && f <= 99 {
			autoSell = f
		} else {
			m.logger.Warn(fmt.Sprintf("⚠️  Invalid percent_to_sell '%s', defaulting to 99%%: %v", s, err))
		}
	}

	return &Task{
		ID:              line - 1,
		TaskName:        get("task_name"),
		Module:          get("module"),
		WalletName:      get("wallet"),
		Operation:       op,
		AmountSol:       amount,
		SlippagePercent: slippage,
		PriorityFeeSol:  priority,
		ComputeUnits:    computeUnits,
		AutosellAmount:  autoSell,
		TokenMint:       get("token_mint"),
		CreatedAt:       time.Now(),
	}, nil
}

func parseUint32FieldStr(s string) (uint32, error) {
	if s == "" {
		return 0, nil
	}
	u, err := strconv.ParseUint(s, 10, 32)
	return uint32(u), err
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

func clamp(val, min, max, def float64) float64 {
	if val < min || val > max {
		return def
	}
	return val
}
