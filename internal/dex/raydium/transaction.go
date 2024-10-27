package raydium

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// Serialize сериализует данные инструкции свапа
// Метод Serialize нужно обновить для корректной работы с uint8
func (s *SwapInstructionData) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Записываем Instruction как uint8
	if err := buf.WriteByte(s.Instruction); err != nil {
		return nil, fmt.Errorf("failed to serialize instruction: %w", err)
	}

	// Записываем AmountIn и MinAmountOut как uint64
	for _, v := range []uint64{s.AmountIn, s.MinAmountOut} {
		if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
			return nil, fmt.Errorf("failed to serialize value: %w", err)
		}
	}

	return buf.Bytes(), nil
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

	ammID, err := validatePublicKey(poolInfo.AmmID)
	if err != nil {
		logger.Error("Invalid AmmID", zap.Error(err))
		return nil, fmt.Errorf("invalid AmmID: %w", err)
	}

	// Создаем массив для всех аккаунтов, которые нужно проверить
	accountChecks := []struct {
		name    string
		address string
	}{
		{"AmmAuthority", poolInfo.AmmAuthority},
		{"AmmOpenOrders", poolInfo.AmmOpenOrders},
		{"AmmTargetOrders", poolInfo.AmmTargetOrders},
		{"PoolCoinTokenAccount", poolInfo.PoolCoinTokenAccount},
		{"PoolPcTokenAccount", poolInfo.PoolPcTokenAccount},
		{"SerumProgramID", poolInfo.SerumProgramID},
		{"SerumMarket", poolInfo.SerumMarket},
		{"SerumBids", poolInfo.SerumBids},
		{"SerumAsks", poolInfo.SerumAsks},
		{"SerumEventQueue", poolInfo.SerumEventQueue},
		{"SerumCoinVaultAccount", poolInfo.SerumCoinVaultAccount},
		{"SerumPcVaultAccount", poolInfo.SerumPcVaultAccount},
		{"SerumVaultSigner", poolInfo.SerumVaultSigner},
	}

	// Создаем слайс для аккаунтов с предварительно выделенной памятью
	accounts := make([]*solana.AccountMeta, 0, len(accountChecks)+7) // +7 для базовых аккаунтов

	// Добавляем базовые аккаунты
	accounts = append(accounts, []*solana.AccountMeta{
		{PublicKey: userSourceTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: userDestinationTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: ammID, IsSigner: false, IsWritable: true},
	}...)

	// Проверяем и добавляем остальные аккаунты
	for _, check := range accountChecks {
		pubKey, err := validatePublicKey(check.address)
		if err != nil {
			logger.Error(fmt.Sprintf("Invalid %s", check.name),
				zap.String("address", check.address),
				zap.Error(err))
			return nil, fmt.Errorf("invalid %s: %w", check.name, err)
		}

		isWritable := false
		switch check.name {
		case "AmmOpenOrders", "AmmTargetOrders", "PoolCoinTokenAccount",
			"PoolPcTokenAccount", "SerumMarket", "SerumBids", "SerumAsks",
			"SerumEventQueue", "SerumCoinVaultAccount", "SerumPcVaultAccount":
			isWritable = true
		}

		accounts = append(accounts, &solana.AccountMeta{
			PublicKey:  pubKey,
			IsSigner:   false,
			IsWritable: isWritable,
		})
	}

	// Добавляем системные аккаунты
	accounts = append(accounts, []*solana.AccountMeta{
		{PublicKey: userWallet, IsSigner: true, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarClockPubkey, IsSigner: false, IsWritable: false},
	}...)

	// Создание данных инструкции
	instructionData := SwapInstructionData{
		Instruction:  poolInfo.RaydiumSwapInstructionCode,
		AmountIn:     amountIn,
		MinAmountOut: minAmountOut,
	}

	data, err := instructionData.Serialize()
	if err != nil {
		logger.Error("Failed to serialize instruction data", zap.Error(err))
		return nil, fmt.Errorf("failed to serialize instruction data: %w", err)
	}

	instruction := solana.NewInstruction(ammProgramID, accounts, data)

	logger.Debug("Swap instruction created successfully")
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
