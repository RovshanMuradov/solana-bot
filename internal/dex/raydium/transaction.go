// internal/dex/raydium/transaction.go
package raydium

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/computebudget"
	"github.com/rovshanmuradov/solana-bot/internal/transaction"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// SwapInstructionData представляет данные инструкции свапа
type SwapInstructionData struct {
	Instruction  uint64 // Код инструкции
	AmountIn     uint64 // Сумма входа
	MinAmountOut uint64 // Минимальная сумма выхода
}

// RaydiumPoolInfo содержит информацию о пуле Raydium
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

// Метод Serialize для SwapInstructionData
func (s *SwapInstructionData) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Пишем поля структуры в буфер
	if err := binary.Write(buf, binary.LittleEndian, s.Instruction); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, s.AmountIn); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, s.MinAmountOut); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// CreateSwapInstruction создает инструкцию свапа для Raydium
func (r *RaydiumDEX) CreateSwapInstruction(
	userWallet solana.PublicKey,
	userSourceTokenAccount solana.PublicKey,
	userDestinationTokenAccount solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
	poolInfo *RaydiumPoolInfo,
) (solana.Instruction, error) {
	ammProgramID := solana.MustPublicKeyFromBase58(poolInfo.AmmProgramID)

	// Определение AccountMeta
	accounts := []*solana.AccountMeta{
		{PublicKey: userSourceTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: userDestinationTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmID), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmAuthority), IsSigner: false, IsWritable: false},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmOpenOrders), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.AmmTargetOrders), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.PoolCoinTokenAccount), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.PoolPcTokenAccount), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumProgramID), IsSigner: false, IsWritable: false},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumMarket), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumBids), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumAsks), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumEventQueue), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumCoinVaultAccount), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumPcVaultAccount), IsSigner: false, IsWritable: true},
		{PublicKey: solana.MustPublicKeyFromBase58(poolInfo.SerumVaultSigner), IsSigner: false, IsWritable: false},
		{PublicKey: userWallet, IsSigner: true, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarClockPubkey, IsSigner: false, IsWritable: false},
	}

	// Создание данных инструкции
	instructionData := SwapInstructionData{
		Instruction:  poolInfo.RaydiumSwapInstructionCode,
		AmountIn:     amountIn,
		MinAmountOut: minAmountOut,
	}

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

// PrepareAndSendTransaction готовит и отправляет транзакцию свапа
func (r *RaydiumDEX) PrepareAndSendTransaction(
	ctx context.Context,
	task *types.Task,
	wallet *wallet.Wallet,
	logger *zap.Logger,
) error {
	// Получение последнего blockhash
	recentBlockhash, err := r.client.GetRecentBlockhash(ctx)
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
	swapInstruction, err := r.CreateSwapInstruction(
		wallet.PublicKey,
		task.UserSourceTokenAccount,
		task.UserDestinationTokenAccount,
		amountIn,
		minAmountOut,
		logger,
		r.poolInfo,
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

	// Использование функции RetryOperation из пакета transaction
	err = transaction.RetryOperation(3, time.Second, func() error {
		signature, err := r.client.SendTransaction(ctx, tx)
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
