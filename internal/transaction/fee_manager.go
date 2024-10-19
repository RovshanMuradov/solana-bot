// internal/transaction/fee_manager.go
package transaction

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/computebudget"
	"go.uber.org/zap"
)

// В функции PrepareAndSendTransaction перед созданием основной инструкции добавьте инструкцию приоритета
func AdjustPriorityFee(tx *solana.Transaction, priorityFee uint64, logger *zap.Logger) {
	// Создание инструкции ComputeBudget для увеличения приоритета
	priorityInstruction := computebudget.RequestHeapFrame{
		Bytes: 256 * 1024, // Пример значения, настройте по необходимости
	}.Build()

	// Вставка инструкции в начало транзакции
	tx.Message.Instructions = append([]solana.CompiledInstruction{priorityInstruction.MessageInstruction}, tx.Message.Instructions...)
}
