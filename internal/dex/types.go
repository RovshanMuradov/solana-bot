// internal/dex/types.go
package dex

// OperationType определяет тип операции DEX.
type OperationType string

const (
	// OperationSnipe – операция покупки (snipe).
	OperationSnipe OperationType = "snipe"
	// OperationSell – операция продажи.
	OperationSell OperationType = "sell"
	// OperationSwap – операция свопа (на Raydium).
	OperationSwap OperationType = "swap"
)

// Task представляет задачу (операцию) для DEX.
type Task struct {
	// Operation – тип операции (snipe, sell, swap).
	Operation OperationType
	// Amount – сумма операции (например, количество токенов в лампортах).
	Amount uint64
	// MinSolOutput – для snipe: максимальная цена (в лампортах SOL), для sell: минимальный ожидаемый вывод SOL.
	MinSolOutput uint64
}
