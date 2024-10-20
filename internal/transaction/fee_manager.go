// // internal/transaction/fee_manager.go
package transaction

// import (
// 	"fmt"

// 	"github.com/gagliardetto/solana-go"
// 	"github.com/gagliardetto/solana-go/programs/computebudget"
// )

// // В функции PrepareAndSendTransaction перед созданием основной инструкции добавьте инструкцию приоритета
// // Пример корректного способа установки приоритетной комиссии (если поддерживается)
// func AdjustPriorityFee(tx *solana.Transaction, priorityFee float64) error {
// 	if tx == nil {
// 		return fmt.Errorf("transaction is nil")
// 	}

// 	priorityFeeLamports := uint64(priorityFee * 1e9) // 1 SOL = 1e9 lamports

// 	// Используем библиотеку computebudget для создания инструкции установки приоритетной комиссии
// 	computeBudgetInstruction := computebudget.NewSetComputeUnitPrice(
// 		priorityFeeLamports,
// 	).Build()

// 	tx.Message.Instructions = append([]solana.CompiledInstruction{computeBudgetInstruction}, tx.Message.Instructions...)

// 	return nil
// }
