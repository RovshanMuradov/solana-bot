// internal/sniping/strategy.go
package sniping

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/rovshanmuradov/solana-bot/internal/types"
)

func LoadTasks(path string) ([]*types.Task, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var tasks []*types.Task
	for _, record := range records[1:] { // Пропускаем заголовок
		task, err := parseTask(record)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func parseTask(record []string) (*types.Task, error) {
	if len(record) != 16 {
		return nil, errors.New("invalid CSV format")
	}

	workers, err := strconv.Atoi(record[2])
	if err != nil {
		return nil, fmt.Errorf("invalid Workers value: %v", err)
	}

	delta, err := strconv.Atoi(record[4])
	if err != nil {
		return nil, fmt.Errorf("invalid Delta value: %v", err)
	}

	priorityFee, err := strconv.ParseFloat(record[5], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid PriorityFee value: %v", err)
	}

	amountIn, err := strconv.ParseFloat(record[9], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid AmountIn value: %v", err)
	}

	// Парсим слиппаж
	slippageConfig, err := types.NewSlippageConfig(record[10])
	if err != nil {
		return nil, fmt.Errorf("invalid slippage configuration: %w", err)
	}

	autosellPercent, err := strconv.ParseFloat(record[11], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid AutosellPercent value: %v", err)
	}

	autosellDelay, err := strconv.Atoi(record[12])
	if err != nil {
		return nil, fmt.Errorf("invalid AutosellDelay value: %v", err)
	}

	autosellAmount, err := strconv.ParseFloat(record[13], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid AutosellAmount value: %v", err)
	}

	transactionDelay, err := strconv.Atoi(record[14])
	if err != nil {
		return nil, fmt.Errorf("invalid TransactionDelay value: %v", err)
	}

	autosellPriorityFee, err := strconv.ParseFloat(record[15], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid AutosellPriorityFee value: %v", err)
	}

	// Устанавливаем DEXName
	dexName := "Raydium" // По умолчанию используем Raydium
	if record[1] == "pump.fun" {
		dexName = "Pump.fun"
	}

	return &types.Task{
		TaskName:            record[0],
		Module:              record[1],
		Workers:             workers,
		WalletName:          record[3],
		Delta:               delta,
		PriorityFee:         priorityFee,
		AMMID:               record[6],
		SourceToken:         record[7],
		TargetToken:         record[8],
		AmountIn:            amountIn,
		AutosellPercent:     autosellPercent,
		AutosellDelay:       autosellDelay,
		AutosellAmount:      autosellAmount,
		TransactionDelay:    transactionDelay,
		AutosellPriorityFee: autosellPriorityFee,
		DEXName:             dexName,
		SlippageConfig:      slippageConfig,
	}, nil
}
