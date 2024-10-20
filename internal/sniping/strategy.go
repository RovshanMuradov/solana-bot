package sniping

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/gagliardetto/solana-go"
)

type Task struct {
	TaskName                    string
	Module                      string
	Workers                     int
	WalletName                  string
	Delta                       int
	PriorityFee                 float64
	AMMID                       string
	SourceToken                 string
	TargetToken                 string
	AmountIn                    float64
	MinAmountOut                float64
	AutosellPercent             float64
	AutosellDelay               int
	AutosellAmount              float64
	TransactionDelay            int
	AutosellPriorityFee         float64
	UserSourceTokenAccount      solana.PublicKey
	UserDestinationTokenAccount solana.PublicKey
	SourceTokenDecimals         int
	TargetTokenDecimals         int
}

func LoadTasks(path string) ([]*Task, error) {
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

	var tasks []*Task
	for _, record := range records[1:] { // Пропускаем заголовок
		task, err := parseTask(record)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func parseTask(record []string) (*Task, error) {
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

	minAmountOut, err := strconv.ParseFloat(record[10], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid MinAmountOut value: %v", err)
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

	return &Task{
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
		MinAmountOut:        minAmountOut,
		AutosellPercent:     autosellPercent,
		AutosellDelay:       autosellDelay,
		AutosellAmount:      autosellAmount,
		TransactionDelay:    transactionDelay,
		AutosellPriorityFee: autosellPriorityFee,
	}, nil
}
