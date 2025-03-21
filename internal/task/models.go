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
	"time"

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
	AmountSol           float64 // For buy: Amount in SOL to spend. For sell: Number of tokens to sell
	SlippagePercent     float64 // Slippage tolerance in percent (0-100)
	PriorityFeeSol      string  // Priority fee in SOL (string format, e.g. "0.000001")
	ComputeUnits        uint32  // Compute units for the transaction
	ContractOrTokenMint string
	MonitorInterval     string // Interval for price monitoring (format: "5s", "1m", etc.)
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
	// Default monitor interval is 5 seconds if not specified
	monitorInterval := "5s"
	if t.MonitorInterval != "" {
		monitorInterval = t.MonitorInterval
	}

	// Parse the interval duration
	interval, err := time.ParseDuration(monitorInterval)
	if err != nil {
		// Fallback to default if parsing fails
		interval = 5 * time.Second
	}

	return &dex.Task{
		Operation:       dex.OperationType(t.Operation),
		AmountSol:       t.AmountSol,
		SlippagePercent: t.SlippagePercent,
		TokenMint:       t.ContractOrTokenMint,
		PriorityFee:     t.PriorityFeeSol,
		ComputeUnits:    t.ComputeUnits,
		MonitorInterval: interval,
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
// CSV format: task_name,module,wallet,operation,amount,slippage_percent,priority_fee_sol,contract_address,compute_units,monitor_interval
// For buy operations, amount = SOL to spend
// For sell operations, amount = number of tokens to sell
// monitor_interval is an optional field for the monitoring interval (e.g., "5s", "1m")
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

		slippagePercent, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			m.logger.Warn("Invalid slippage_percent value", zap.String("value", row[5]), zap.Error(err))
			continue
		}

		// Validate slippage is between 0 and 100
		if slippagePercent < 0 || slippagePercent > 100 {
			m.logger.Warn("Slippage percent out of range (0-100)", zap.Float64("slippage", slippagePercent))
			// Default to 1% if out of range
			slippagePercent = 1.0
		}

		// Parse priority fee as string (in SOL)
		priorityFeeSol := row[6]
		if priorityFeeSol == "" {
			priorityFeeSol = "default" // Use default if not specified
		}

		// Parse compute units (default is 0 which means use default value)
		var computeUnits uint32
		if len(row) > 8 && row[8] != "" {
			computeUnitsUint64, err := strconv.ParseUint(row[8], 10, 32)
			if err != nil {
				m.logger.Warn("Invalid compute_units value", zap.String("value", row[8]), zap.Error(err))
			} else {
				computeUnits = uint32(computeUnitsUint64)
			}
		}
		// Get monitor interval if available
		monitorInterval := ""
		if len(row) > 9 && row[9] != "" {
			monitorInterval = row[9]
		}

		tasks = append(tasks, Task{
			TaskName:            row[0],
			Module:              row[1],
			WalletName:          row[2],
			Operation:           row[3],
			AmountSol:           amountSol,
			SlippagePercent:     slippagePercent,
			PriorityFeeSol:      priorityFeeSol,
			ComputeUnits:        computeUnits,
			ContractOrTokenMint: row[7],
			MonitorInterval:     monitorInterval,
		})
	}

	m.logger.Info("Tasks loaded successfully", zap.Int("count", len(tasks)))
	return tasks, nil
}
