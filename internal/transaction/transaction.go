// internal/transaction/transaction.go
package transaction

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/computebudget"
	solanaclient "github.com/rovshanmuradov/solana-bot/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/sniping"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// SwapInstructionData представляет данные инструкции свапа
type SwapInstructionData struct {
	Instruction  uint64 // Код инструкции
	AmountIn     uint64 // Сумма входа
	MinAmountOut uint64 // Минимальная сумма выхода
}

type RaydiumPoolInfo struct {
	AmmProgramID               string // Program ID AMM Raydium
	AmmID                      string // AMM ID пула
	AmmAuthority               string // Авторитет AMM
	AmmOpenOrders              string // Открытые ордера AMM
	AmmTargetOrders            string // Целевые ордера AMM
	PoolCoinTokenAccount       string // Аккаунт токена пула
	PoolPcTokenAccount         string // Аккаунт токена PC пула
	SerumProgramID             string // Program ID Serum DEX
	SerumMarket                string // Рынок Serum
	SerumBids                  string // Заявки на покупку Serum
	SerumAsks                  string // Заявки на продажу Serum
	SerumEventQueue            string // Очередь событий Serum
	SerumCoinVaultAccount      string // Аккаунт хранилища монет
	SerumPcVaultAccount        string // Аккаунт хранилища PC
	SerumVaultSigner           string // Подписант хранилища Serum
	RaydiumSwapInstructionCode uint64 // Код инструкции свапа Raydium
}

const (
	RaydiumAmmProgramID  = "AMMProgramID"   // Замените на реальный Program ID AMM Raydium
	RaydiumAmmID         = "AMM ID"         // Замените на конкретный AMM ID пула
	RaydiumAmmAuthority  = "AMM Authority"  // Замените на AMM Authority
	RaydiumOpenOrders    = "Open Orders"    // Замените на Open Orders Address
	RaydiumTargetOrders  = "Target Orders"  // Замените на Target Orders Address
	RaydiumPoolCoinToken = "Pool Coin"      // Замените на Pool Coin Address
	RaydiumPoolPcToken   = "Pool PC"        // Замените на Pool PC Address
	SerumProgramID       = "SerumProgramID" // Замените на Program ID Serum DEX
)

func RetryOperation(attempts int, sleep time.Duration, operation func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = operation()
		if err == nil {
			return nil
		}
		time.Sleep(sleep)
		sleep *= 2 // Экспоненциальное увеличение задержки
	}
	return fmt.Errorf("после %d попыток: %w", attempts, err)
}

func PrepareAndSendTransaction(
	ctx context.Context,
	task *sniping.Task,
	wallet *wallet.Wallet,
	client *solanaclient.Client,
	logger *zap.Logger,
	poolInfo *RaydiumPoolInfo,
) error {
	// Получение последнего blockhash
	recentBlockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		logger.Error("Failed to get recent blockhash", zap.Error(err))
		return err
	}

	// Преобразование AmountIn и MinAmountOut в uint64 с учетом десятичных знаков токена
	amountIn := uint64(task.AmountIn * math.Pow10(task.SourceTokenDecimals))
	minAmountOut := uint64(task.MinAmountOut * math.Pow10(task.TargetTokenDecimals))

	// Создание инструкции ComputeBudget для установки приоритетной комиссии
	priorityFee := uint64(task.PriorityFee * 1e9) // Конвертация SOL в лампорты
	computeBudgetInstruction := computebudget.NewSetComputeUnitPrice(
		priorityFee, // Устанавливает цену за единицу вычислений
	).Build()

	// Создание инструкции свапа
	swapInstruction, err := CreateRaydiumSwapInstruction(
		wallet.PublicKey,
		task.UserSourceTokenAccount,
		task.UserDestinationTokenAccount,
		amountIn,
		minAmountOut,
		logger,
		poolInfo,
	)
	if err != nil {
		logger.Error("Failed to create swap instruction", zap.Error(err))
		return err
	}

	// Создание транзакции
	tx, err := solana.NewTransaction(
		[]solana.Instruction{computeBudgetInstruction, swapInstruction},
		recentBlockhash,
	)
	if err != nil {
		logger.Error("Failed to create transaction", zap.Error(err))
		return err
	}

	// Подписание транзакции
	_, err = tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(wallet.PublicKey) {
				return &wallet.PrivateKey
			}
			return nil
		},
	)
	if err != nil {
		logger.Error("Failed to sign transaction", zap.Error(err))
		return err
	}

	// Использование в PrepareAndSendTransaction:
	err = RetryOperation(3, time.Second, func() error {
		signature, err := client.SendTransaction(ctx, tx)
		if err != nil {
			logger.Warn("Failed to send transaction, retrying", zap.Error(err))
			return err
		}
		logger.Info("Transaction sent successfully", zap.String("signature", signature.String()))
		return nil
	})
	if err != nil {
		logger.Error("All attempts to send transaction failed", zap.Error(err))
		return err
	}
	return nil
}

// Метод Serialize для SwapInstructionData
func (s *SwapInstructionData) Serialize() ([]byte, error) {
	// Создаем буфер для данных
	buf := new(bytes.Buffer)

	// Пишем код инструкции
	err := binary.Write(buf, binary.LittleEndian, s.Instruction)
	if err != nil {
		return nil, err
	}

	// Пишем AmountIn
	err = binary.Write(buf, binary.LittleEndian, s.AmountIn)
	if err != nil {
		return nil, err
	}

	// Пишем MinAmountOut
	err = binary.Write(buf, binary.LittleEndian, s.MinAmountOut)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func CreateRaydiumSwapInstruction(
	userWallet solana.PublicKey,
	userSourceTokenAccount solana.PublicKey,
	userDestinationTokenAccount solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
	poolInfo *RaydiumPoolInfo,
) (solana.Instruction, error) {
	// Установка Program ID Raydium
	ammProgramID := solana.MustPublicKeyFromBase58(poolInfo.AmmProgramID)

	// Определение AccountMeta
	accounts := []*solana.AccountMeta{
		// User Source Token Account
		{PublicKey: userSourceTokenAccount, IsSigner: false, IsWritable: true},
		// User Destination Token Account
		{PublicKey: userDestinationTokenAccount, IsSigner: false, IsWritable: true},
		// AMM ID
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmID), IsSigner: false, IsWritable: true},
		// AMM Authority
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmAuthority), IsSigner: false, IsWritable: false},
		// AMM Open Orders
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmOpenOrders), IsSigner: false, IsWritable: true},
		// AMM Target Orders
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmTargetOrders), IsSigner: false, IsWritable: true},
		// Pool Coin Token Account
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.PoolCoinTokenAccount), IsSigner: false, IsWritable: true},
		// Pool PC Token Account
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.PoolPcTokenAccount), IsSigner: false, IsWritable: true},
		// Serum Program ID
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumProgramID), IsSigner: false, IsWritable: false},
		// Serum Market
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumMarket), IsSigner: false, IsWritable: true},
		// Serum Bids
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumBids), IsSigner: false, IsWritable: true},
		// Serum Asks
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumAsks), IsSigner: false, IsWritable: true},
		// Serum Event Queue
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumEventQueue), IsSigner: false, IsWritable: true},
		// Serum Coin Vault Account
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumCoinVaultAccount), IsSigner: false, IsWritable: true},
		// Serum PC Vault Account
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumPcVaultAccount), IsSigner: false, IsWritable: true},
		// Serum Vault Signer
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumVaultSigner), IsSigner: false, IsWritable: false},
		// User Wallet (плательщик комиссии)
		{PublicKey: userWallet, IsSigner: true, IsWritable: false},
		// Token Program ID
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		// Sysvar Rent
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		// Sysvar Clock
		{PublicKey: solana.SysVarClockPubkey, IsSigner: false, IsWritable: false},
	}

	// Создание данных инструкции
	instructionData := SwapInstructionData{
		Instruction:  poolInfo.RaydiumSwapInstructionCode, // Предполагается, что это поле добавлено в RaydiumPoolInfo
		AmountIn:     amountIn,
		MinAmountOut: minAmountOut,
	}

	// Сериализация данных инструкции
	data, err := instructionData.Serialize()
	if err != nil {
		logger.Error("Failed to serialize instruction data", zap.Error(err))
		return nil, err
	}

	// Создание инструкции
	instruction := solana.NewInstruction(
		ammProgramID,
		accounts,
		data,
	)

	return instruction, nil
}

// В функции PrepareAndSendTransaction перед созданием основной инструкции добавьте инструкцию приоритета
// Пример корректного способа установки приоритетной комиссии (если поддерживается)
func AdjustPriorityFee(tx *solana.Transaction, priorityFee float64) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	priorityFeeLamports := uint64(priorityFee * 1e9) // 1 SOL = 1e9 lamports

	// Используем библиотеку computebudget для создания инструкции установки приоритетной комиссии
	computeBudgetInstruction := computebudget.NewSetComputeUnitPrice(
		priorityFeeLamports,
	).Build()

	tx.Message.Instructions = append([]solana.CompiledInstruction{computeBudgetInstruction}, tx.Message.Instructions...)

	return nil
}
