// =============================================
// File: internal/task/models.go
// =============================================
// Package task provides task management functionality
package task

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
)

// Constants
const LamportsPerSOL = 1_000_000_000

// Task represents a trading task from CSV configuration
type Task struct {
	TaskName            string
	Module              string
	WalletName          string
	Operation           string
	AmountSol           float64
	MinSolOutputSol     float64
	PriorityFeeLamports uint64
	ContractOrTokenMint string
}

// Config defines application configuration
type Config struct {
	RPCList      []string `mapstructure:"rpc_list"`
	WebSocketURL string   `mapstructure:"websocket_url"`
	PostgresURL  string   `mapstructure:"postgres_url"`
	DebugLogging bool     `mapstructure:"debug_logging"`
}

// LoadConfig reads and validates configuration
func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetDefault("debug_logging", true)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config error: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	// Basic validation
	if len(cfg.RPCList) == 0 {
		return nil, fmt.Errorf("rpc_list is empty")
	}
	if cfg.PostgresURL == "" {
		return nil, fmt.Errorf("postgres_url is required")
	}

	return &cfg, nil
}

// ToDEXTask converts Task to dex.Task format
func (t *Task) ToDEXTask() *dex.Task {
	return &dex.Task{
		Operation:    dex.OperationType(t.Operation),
		Amount:       uint64(t.AmountSol * LamportsPerSOL),
		MinSolOutput: uint64(t.MinSolOutputSol * LamportsPerSOL),
		TokenMint:    t.ContractOrTokenMint,
	}
}

// Manager handles task loading and processing
type Manager struct {
	logger *zap.Logger
}

// NewManager creates a new task manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{logger: logger}
}

// LoadTasks reads tasks from CSV file
// CSV format: task_name,module,wallet,operation,amount_sol,min_sol_output_sol,priority_fee_lamports,contract_address
func (m *Manager) LoadTasks(path string) ([]Task, error) {
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
	tasks := make([]Task, 0, len(records)-1)

	for i, row := range records[1:] {
		if len(row) < 8 {
			m.logger.Warn("Skipping row with insufficient columns",
				zap.Int("row", i+2),
				zap.Int("columns", len(row)))
			continue
		}

		// Parse numeric values
		amountSol, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			m.logger.Warn("Invalid amount_sol value", zap.String("value", row[4]), zap.Error(err))
			continue // Skip invalid rows instead of setting zeros
		}

		minSolOut, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			m.logger.Warn("Invalid min_sol_output_sol value", zap.String("value", row[5]), zap.Error(err))
			continue
		}

		priority, err := strconv.ParseUint(row[6], 10, 64)
		if err != nil {
			m.logger.Warn("Invalid priority_fee_lamports value", zap.String("value", row[6]), zap.Error(err))
			continue
		}

		tasks = append(tasks, Task{
			TaskName:            row[0],
			Module:              row[1],
			WalletName:          row[2],
			Operation:           row[3],
			AmountSol:           amountSol,
			MinSolOutputSol:     minSolOut,
			PriorityFeeLamports: priority,
			ContractOrTokenMint: row[7],
		})
	}

	m.logger.Info("Tasks loaded successfully", zap.Int("count", len(tasks)))
	return tasks, nil
}
