// =============================================
// File: internal/task/models.go
// =============================================
package task

import (
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/spf13/viper"
	"os"
	"strconv"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
)

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

type Config struct {
	RPCList      []string `mapstructure:"rpc_list"`
	WebSocketURL string   `mapstructure:"websocket_url"`
	PostgresURL  string   `mapstructure:"postgres_url"`

	// Add other small flags if necessary
	DebugLogging bool `mapstructure:"debug_logging"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Defaults
	v.SetDefault("debug_logging", true)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Minimal validation
	if len(cfg.RPCList) == 0 {
		return nil, errors.New("rpc_list is empty, please specify at least one Solana RPC")
	}
	if cfg.PostgresURL == "" {
		return nil, errors.New("postgres_url is required")
	}

	return &cfg, nil
}

// ToDEXTask converts a Task to the dex.Task format used by DEX adapters
func (t *Task) ToDEXTask() *dex.Task {
	// Convert SOL amounts to lamports (1 SOL = 10^9 lamports)
	amountLamports := uint64(t.AmountSol * 1e9)
	minLamports := uint64(t.MinSolOutputSol * 1e9)

	return &dex.Task{
		Operation:    dex.OperationType(t.Operation),
		Amount:       amountLamports,
		MinSolOutput: minLamports,
		TokenMint:    t.ContractOrTokenMint,
	}
}

// Manager handles task loading and processing
type Manager struct {
	logger *zap.Logger
}

// NewManager creates a new task manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// LoadTasks reads tasks from a CSV file
// The CSV should have columns:
// task_name,module,wallet,operation,amount_sol,min_sol_output_sol,priority_fee_lamports,contract_address
func (m *Manager) LoadTasks(path string) ([]Task, error) {
	m.logger.Debug("Loading tasks from CSV", zap.String("path", path))

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open tasks file: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("no tasks found in CSV")
	}

	var tasks []Task
	// Skip header row
	for _, row := range records[1:] {
		if len(row) < 8 {
			m.logger.Warn("Skipping row with insufficient columns", zap.Int("columns", len(row)))
			continue
		}

		amountSol, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			m.logger.Warn("Invalid amount_sol value", zap.String("value", row[4]), zap.Error(err))
			amountSol = 0
		}

		minSolOut, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			m.logger.Warn("Invalid min_sol_output_sol value", zap.String("value", row[5]), zap.Error(err))
			minSolOut = 0
		}

		priority, err := strconv.ParseUint(row[6], 10, 64)
		if err != nil {
			m.logger.Warn("Invalid priority_fee_lamports value", zap.String("value", row[6]), zap.Error(err))
			priority = 0
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
