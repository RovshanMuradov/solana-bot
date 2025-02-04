// ======================================
// File: internal/sniping/sniper.go
// ======================================
package sniping

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
)

// LoadTasks reads tasks from task.csv and converts them to dex.Task objects.
// Adjust parsing logic as needed for your CSV structure.
func LoadTasks(filePath string) ([]*dex.Task, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cannot read CSV file: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file is empty or has no data rows")
	}

	var tasks []*dex.Task
	// Example: we expect columns like: [TaskName, Module, ... , AmountIn, ...]
	// Adjust indices based on your CSV layout
	for _, row := range records[1:] {
		if len(row) < 9 {
			continue
		}
		// row[0] = TaskName
		// row[1] = Module (e.g. "pump.fun" or "Raydium")
		// row[8] = AmountIn (example index)

		operation := dex.OperationSnipe
		taskName := strings.ToLower(row[0])
		// Example: if CSV has "sellTokens" or "swapTokens", pick operation accordingly
		if strings.Contains(taskName, "sell") {
			operation = dex.OperationSell
		} else if strings.Contains(taskName, "swap") {
			operation = dex.OperationSwap
		}

		amountFloat, errConv := strconv.ParseFloat(row[8], 64)
		if errConv != nil {
			continue
		}
		// Convert SOL (0.02) to lamports if needed. Here we assume user wrote direct lamports or any other logic.
		amountLamports := uint64(amountFloat * 1e9)

		// As an example, MinSolOutput = 0 if not specified
		task := &dex.Task{
			Operation:    operation,
			Amount:       amountLamports,
			MinSolOutput: 0, // or parse from CSV if there's a column for it
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}
