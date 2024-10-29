package raydium

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// Serialize сериализует данные инструкции свапа
func (s *SwapInstructionData) Serialize() ([]byte, error) {
	data := make([]byte, 17)

	// Instruction (1 byte)
	data[0] = s.Instruction

	// AmountIn (8 bytes)
	binary.LittleEndian.PutUint64(data[1:9], s.AmountIn)

	// MinAmountOut (8 bytes)
	binary.LittleEndian.PutUint64(data[9:17], s.MinAmountOut)

	return data, nil
}

// TestSwapInstructionDataSerialization тест для проверки сериализации
func TestSwapInstructionDataSerialization(t *testing.T) {
	inst := &SwapInstructionData{
		Instruction:  1,
		AmountIn:     20000000,
		MinAmountOut: 6,
	}

	data, err := inst.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	// Проверяем instruction code
	if data[0] != 1 {
		t.Errorf("Expected instruction 1, got %d", data[0])
	}

	// Проверяем amountIn
	gotAmountIn := binary.LittleEndian.Uint64(data[1:9])
	if gotAmountIn != 20000000 {
		t.Errorf("Expected amountIn 20000000, got %d", gotAmountIn)
	}

	// Проверяем minAmountOut
	gotMinAmountOut := binary.LittleEndian.Uint64(data[9:17])
	if gotMinAmountOut != 6 {
		t.Errorf("Expected minAmountOut 6, got %d", gotMinAmountOut)
	}
}

// Debug выводит шестнадцатеричное представление данных
func (s *SwapInstructionData) Debug(logger *zap.Logger) {
	data, err := s.Serialize()
	if err != nil {
		logger.Error("Failed to serialize for debug", zap.Error(err))
		return
	}

	// Проверяем данные
	amountIn := binary.LittleEndian.Uint64(data[1:9])
	minAmountOut := binary.LittleEndian.Uint64(data[9:17])

	logger.Debug("Instruction data debug",
		zap.Uint8("instruction", data[0]),
		zap.Uint64("amount_in_original", s.AmountIn),
		zap.Uint64("amount_in_serialized", amountIn),
		zap.Uint64("min_amount_out_original", s.MinAmountOut),
		zap.Uint64("min_amount_out_serialized", minAmountOut),
		zap.Binary("raw_data", data))
}

// validatePublicKey проверяет корректность публичного ключа
func validatePublicKey(key string) (solana.PublicKey, error) {
	if key == "" {
		return solana.PublicKey{}, fmt.Errorf("empty public key")
	}

	pubKey, err := solana.PublicKeyFromBase58(key)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("invalid public key %s: %w", key, err)
	}

	return pubKey, nil
}

// CreateSwapInstruction создает инструкцию свапа для Raydium
func (r *DEX) CreateSwapInstruction(
	userWallet solana.PublicKey,
	userSourceTokenAccount solana.PublicKey,
	userDestinationTokenAccount solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
	poolInfo *Pool,
) (solana.Instruction, error) {
	logger.Debug("Creating swap instruction",
		zap.String("user_wallet", userWallet.String()),
		zap.String("source_account", userSourceTokenAccount.String()),
		zap.String("destination_account", userDestinationTokenAccount.String()),
		zap.Uint64("amount_in", amountIn),
		zap.Uint64("min_amount_out", minAmountOut))

	if poolInfo == nil {
		return nil, fmt.Errorf("pool info is nil")
	}

	// Проверяем и конвертируем все необходимые публичные ключи
	ammProgramID, err := validatePublicKey(poolInfo.AmmProgramID)
	if err != nil {
		logger.Error("Invalid AmmProgramID", zap.Error(err))
		return nil, fmt.Errorf("invalid AmmProgramID: %w", err)
	}

	// Проверяем и создаем все необходимые аккаунты
	requiredAccounts := map[string]string{
		"AmmID":                poolInfo.AmmID,
		"AmmAuthority":         poolInfo.AmmAuthority,
		"AmmOpenOrders":        poolInfo.AmmOpenOrders,
		"AmmTargetOrders":      poolInfo.AmmTargetOrders,
		"PoolCoinTokenAccount": poolInfo.PoolCoinTokenAccount,
		"PoolPcTokenAccount":   poolInfo.PoolPcTokenAccount,
		"SerumProgramID":       poolInfo.SerumProgramID,
		"SerumMarket":          poolInfo.SerumMarket,
		"SerumBids":            poolInfo.SerumBids,
		"SerumAsks":            poolInfo.SerumAsks,
		"SerumEventQueue":      poolInfo.SerumEventQueue,
		"SerumCoinVault":       poolInfo.SerumCoinVaultAccount,
		"SerumPcVault":         poolInfo.SerumPcVaultAccount,
		"SerumVaultSigner":     poolInfo.SerumVaultSigner,
	}

	accounts := make(map[string]solana.PublicKey)
	for name, address := range requiredAccounts {
		pubKey, err := validatePublicKey(address)
		if err != nil {
			logger.Error(fmt.Sprintf("Invalid %s", name),
				zap.String("address", address),
				zap.Error(err))
			return nil, fmt.Errorf("invalid %s: %w", name, err)
		}
		accounts[name] = pubKey
	}

	// Создаем слайс аккаунтов в правильном порядке для Raydium
	metas := make(solana.AccountMetaSlice, 0, 20)

	// Токен аккаунты пользователя
	metas = append(metas,
		&solana.AccountMeta{PublicKey: userSourceTokenAccount, IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: userDestinationTokenAccount, IsSigner: false, IsWritable: true},
	)

	// Аккаунты AMM
	metas = append(metas,
		&solana.AccountMeta{PublicKey: accounts["AmmID"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["AmmAuthority"], IsSigner: false, IsWritable: false},
		&solana.AccountMeta{PublicKey: accounts["AmmOpenOrders"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["AmmTargetOrders"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["PoolCoinTokenAccount"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["PoolPcTokenAccount"], IsSigner: false, IsWritable: true},
	)

	// Аккаунты Serum
	metas = append(metas,
		&solana.AccountMeta{PublicKey: accounts["SerumProgramID"], IsSigner: false, IsWritable: false},
		&solana.AccountMeta{PublicKey: accounts["SerumMarket"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["SerumBids"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["SerumAsks"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["SerumEventQueue"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["SerumCoinVault"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["SerumPcVault"], IsSigner: false, IsWritable: true},
		&solana.AccountMeta{PublicKey: accounts["SerumVaultSigner"], IsSigner: false, IsWritable: false},
	)

	// Системные аккаунты
	metas = append(metas,
		&solana.AccountMeta{PublicKey: userWallet, IsSigner: true, IsWritable: false},
		&solana.AccountMeta{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		&solana.AccountMeta{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		&solana.AccountMeta{PublicKey: solana.SysVarClockPubkey, IsSigner: false, IsWritable: false},
	)

	// Создание данных инструкции
	instructionData := &SwapInstructionData{
		Instruction:  poolInfo.RaydiumSwapInstructionCode,
		AmountIn:     amountIn,
		MinAmountOut: minAmountOut,
	}

	// Добавляем отладочный вывод
	instructionData.Debug(logger)

	// Сериализация
	data, err := instructionData.Serialize()
	if err != nil {
		logger.Error("Failed to serialize instruction data",
			zap.Error(err),
			zap.Uint8("instruction", instructionData.Instruction),
			zap.Uint64("amount_in", instructionData.AmountIn),
			zap.Uint64("min_amount_out", instructionData.MinAmountOut))
		return nil, fmt.Errorf("failed to serialize instruction data: %w", err)
	}

	// Проверка сериализованных данных
	if len(data) != 17 {
		logger.Error("Invalid serialized data length",
			zap.Int("got_length", len(data)),
			zap.Int("expected_length", 17))
		return nil, fmt.Errorf("invalid serialized data length")
	}

	// Проверяем значения после сериализации
	amountInCheck := binary.LittleEndian.Uint64(data[1:9])
	minAmountOutCheck := binary.LittleEndian.Uint64(data[9:17])

	logger.Debug("Serialized data check",
		zap.Uint64("amount_in_check", amountInCheck),
		zap.Uint64("min_amount_out_check", minAmountOutCheck))

	if amountInCheck != amountIn {
		logger.Error("AmountIn mismatch after serialization",
			zap.Uint64("original", amountIn),
			zap.Uint64("serialized", amountInCheck))
		return nil, fmt.Errorf("amountIn mismatch after serialization")
	}

	instruction := solana.NewInstruction(ammProgramID, metas, data)

	logger.Debug("Created instruction",
		zap.Int("num_accounts", len(metas)),
		zap.Int("data_len", len(data)))

	return instruction, nil
}

// PrepareAndSendTransaction готовит и отправляет транзакцию свапа
func (r *DEX) PrepareAndSendTransaction(
	ctx context.Context,
	task *types.Task,
	userWallet *wallet.Wallet,
	logger *zap.Logger,
	swapInstruction solana.Instruction,
) error {
	recentBlockhash, err := r.client.GetRecentBlockhash(ctx)
	if err != nil {
		logger.Error("Failed to get recent blockhash", zap.Error(err))
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем compute budget инструкции с использованием нового PriorityManager
	priorityManager := types.NewPriorityManager(logger)
	budgetInstructions, err := priorityManager.CreateCustomPriorityInstructions(
		uint64(task.PriorityFee*1e6), // Конвертируем SOL в микро-ламports
		1_000_000,                    // Используем sniping units
	)
	if err != nil {
		logger.Error("Failed to create compute budget instructions", zap.Error(err))
		return fmt.Errorf("failed to create compute budget instructions: %w", err)
	}

	// Combine all instructions properly
	instructions := make([]solana.Instruction, 0, len(budgetInstructions)+1)
	instructions = append(instructions, budgetInstructions...)
	instructions = append(instructions, swapInstruction)

	// Создаем транзакцию
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockhash,
		solana.TransactionPayer(userWallet.PublicKey),
	)
	if err != nil {
		logger.Error("Failed to create transaction", zap.Error(err))
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(userWallet.PublicKey) {
			return &userWallet.PrivateKey
		}
		return nil
	})
	if err != nil {
		logger.Error("Failed to sign transaction", zap.Error(err))
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Отправляем транзакцию
	signature, err := r.client.SendTransaction(ctx, tx)
	if err != nil {
		logger.Error("Failed to send transaction", zap.Error(err))
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	logger.Info("Transaction sent successfully",
		zap.String("signature", signature.String()),
		zap.Float64("priority_fee_sol", task.PriorityFee),
		zap.Uint64("compute_units", 1_000_000))

	return nil
}
