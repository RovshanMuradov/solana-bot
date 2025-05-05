// =============================
// File: internal/dex/pumpfun/accounts.go
// =============================
package pumpfun

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
	"time"
)

// deriveBondingCurveAccounts вычисляет необходимые адреса для взаимодействия
// с bonding curve токена в протоколе Pump.fun.
func (d *DEX) deriveBondingCurveAccounts(_ context.Context) (bondingCurve, associatedBondingCurve solana.PublicKey, err error) {
	// Шаг 1: Вычисление Program Derived Address (PDA) для bonding curve
	bondingCurve, _, err = solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)

	// Шаг 2: Проверка на ошибки при вычислении PDA
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to derive bonding curve: %w", err)
	}

	// Шаг 3: Логирование успешного вычисления адреса bonding curve для отладки
	d.logger.Debug("Derived bonding curve", zap.String("address", bondingCurve.String()))

	// Шаг 4: Вычисление ассоциированного токен-аккаунта (ATA) для bonding curve
	associatedBondingCurve, _, err = solana.FindAssociatedTokenAddress(bondingCurve, d.config.Mint)

	// Шаг 5: Проверка на ошибки при вычислении ATA
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}

	// Шаг 6: Логирование успешного вычисления адреса ATA для отладки
	d.logger.Debug("Derived bonding curve ATA", zap.String("address", associatedBondingCurve.String()))

	// Шаг 7: Возврат вычисленных адресов
	return bondingCurve, associatedBondingCurve, nil
}

// FetchBondingCurveAccount получает и парсит данные аккаунта bonding curve.
func (d *DEX) FetchBondingCurveAccount(ctx context.Context, bondingCurve solana.PublicKey) (*BondingCurve, error) {
	// TODO: delete logging
	// <<< ДОБАВЛЕНО: Логирование времени выполнения RPC >>>
	start := time.Now()
	// Шаг 1: Получение информации об аккаунте с блокчейна
	accountInfo, err := d.client.GetAccountInfo(ctx, bondingCurve)
	// <<< ДОБАВЛЕНО: Логирование времени выполнения RPC >>>
	d.logger.Debug("RPC:GetAccountInfo (BondingCurve)",
		zap.String("account", bondingCurve.String()),
		zap.Duration("took", time.Since(start)),
		zap.Error(err)) // Также логируем ошибку, если она есть

	if err != nil {
		// <<< ДОБАВЛЕНО: Логирование ошибки контекста >>>
		if errors.Is(err, context.Canceled) {
			d.logger.Warn("FetchBondingCurveAccount canceled by context", zap.Error(err), zap.String("account", bondingCurve.String()))
		} else if errors.Is(err, context.DeadlineExceeded) {
			d.logger.Warn("FetchBondingCurveAccount context deadline exceeded", zap.Error(err), zap.String("account", bondingCurve.String()))
		}
		// Шаг 2: Обработка ошибки при неудачном запросе
		return nil, fmt.Errorf("failed to get bonding curve account: %w", err)
	}

	// Шаг 3: Проверка существования аккаунта
	if accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}

	// Шаг 4: Извлечение бинарных данных из аккаунта
	data := accountInfo.Value.Data.GetBinary()

	// Шаг 5: Проверка минимальной длины данных
	if len(data) < 16 {
		return nil, fmt.Errorf("invalid bonding curve data: insufficient length")
	}

	// Шаг 6: Чтение виртуальных резервов токенов (первые 8 байт)
	virtualTokenReserves := binary.LittleEndian.Uint64(data[0:8])

	// Шаг 7: Чтение виртуальных резервов SOL (следующие 8 байт)
	virtualSolReserves := binary.LittleEndian.Uint64(data[8:16])

	// Шаг 8: Создание и возврат структуры с данными
	return &BondingCurve{
		VirtualTokenReserves: virtualTokenReserves,
		VirtualSolReserves:   virtualSolReserves,
	}, nil
}

// FetchGlobalAccount получает и парсит данные глобального аккаунта Pump.fun.
func FetchGlobalAccount(ctx context.Context, client *solbc.Client, globalAddr solana.PublicKey, logger *zap.Logger) (*GlobalAccount, error) {
	// TODO: delete logging
	// <<< ИЗМЕНЕНО: Логирование времени выполнения RPC с использованием переданного логгера >>>
	start := time.Now()
	// Шаг 1: Получение информации об аккаунте с блокчейна
	accountInfo, err := client.GetAccountInfo(ctx, globalAddr)
	// <<< ИЗМЕНЕНО: Логирование времени выполнения RPC >>>
	logger.Debug("RPC:GetAccountInfo (Global)",
		zap.String("account", globalAddr.String()),
		zap.Duration("took", time.Since(start)),
		zap.Error(err))

	if err != nil {
		// <<< ИЗМЕНЕНО: Логирование ошибки контекста с использованием переданного логгера >>>
		if errors.Is(err, context.Canceled) {
			logger.Warn("FetchGlobalAccount canceled by context", zap.Error(err), zap.String("account", globalAddr.String()))
		} else if errors.Is(err, context.DeadlineExceeded) {
			logger.Warn("FetchGlobalAccount context deadline exceeded", zap.Error(err), zap.String("account", globalAddr.String()))
		}
		// Шаг 2: Обработка ошибки при неудачном запросе
		return nil, fmt.Errorf("failed to get global account: %w", err)
	}

	// Шаг 3: Проверка существования аккаунта
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("global account not found: %s", globalAddr.String())
	}

	// Шаг 4: Проверка владельца аккаунта
	if !accountInfo.Value.Owner.Equals(PumpFunProgramID) {
		return nil, fmt.Errorf("global account has incorrect owner: expected %s, got %s",
			PumpFunProgramID.String(), accountInfo.Value.Owner.String())
	}

	// Шаг 5: Извлечение бинарных данных из аккаунта
	data := accountInfo.Value.Data.GetBinary()

	// Шаг 6: Проверка достаточности данных
	// Минимальная длина: 8 (дискриминатор) + 1 (флаг) + 64 (два публичных ключа)
	if len(data) < 8+1+64 {
		return nil, fmt.Errorf("global account data too short: %d bytes", len(data))
	}

	// Шаг 7: Начало десериализации - создание структуры
	account := &GlobalAccount{}

	// Шаг 8: Чтение дискриминатора (8 байт)
	copy(account.Discriminator[:], data[0:8])

	// Шаг 9: Чтение флага инициализации (1 байт)
	account.Initialized = data[8] != 0

	// Шаг 10: Чтение публичного ключа администратора (32 байта)
	authorityBytes := make([]byte, 32)
	copy(authorityBytes, data[9:41])
	account.Authority = solana.PublicKeyFromBytes(authorityBytes)

	// Шаг 11: Чтение публичного ключа получателя комиссий (32 байта)
	feeRecipientBytes := make([]byte, 32)
	copy(feeRecipientBytes, data[41:73])
	account.FeeRecipient = solana.PublicKeyFromBytes(feeRecipientBytes)

	// Шаг 12: Чтение размера комиссии в базовых пунктах (8 байт)
	if len(data) >= 81 {
		account.FeeBasisPoints = binary.LittleEndian.Uint64(data[73:81])
	}

	// Шаг 13: Возврат заполненной структуры
	return account, nil
}
