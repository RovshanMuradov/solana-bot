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

// ----- адреса Bonding‑Curve кэшируются раз‑и‑навсегда -----
func (d *DEX) deriveBondingCurveAccounts(_ context.Context) (solana.PublicKey, solana.PublicKey, error) {
	var initErr error
	d.bcOnce.Do(func() {
		d.bondingCurve, _, initErr = solana.FindProgramAddress(
			[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
			d.config.ContractAddress,
		)
		if initErr != nil {
			return
		}
		d.associatedBondingCurve, _, initErr =
			solana.FindAssociatedTokenAddress(d.bondingCurve, d.config.Mint)
	})
	if initErr != nil {
		return solana.PublicKey{}, solana.PublicKey{},
			fmt.Errorf("failed to derive bonding curve addresses: %w", initErr)
	}
	return d.bondingCurve, d.associatedBondingCurve, nil
}

// ----- новое: берём данные Bonding‑Curve с внутренним TTL‑кэшем -----
const bcCacheTTL = 400 * time.Millisecond

func (d *DEX) getBondingCurveData(ctx context.Context) (*BondingCurve, solana.PublicKey, error) {
	bcAddr, _, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, solana.PublicKey{}, err
	}

	// быстрый путь — свежий кэш
	d.bcCache.mu.RLock()
	if d.bcCache.data != nil && time.Since(d.bcCache.fetchedAt) < bcCacheTTL {
		data := d.bcCache.data
		d.bcCache.mu.RUnlock()
		return data, bcAddr, nil
	}
	d.bcCache.mu.RUnlock()

	// обновляем кэш одним batched‑запросом
	res, err := d.client.GetMultipleAccounts(ctx, []solana.PublicKey{bcAddr})
	if err != nil {
		return nil, bcAddr, err
	}
	if len(res.Value) == 0 || res.Value[0] == nil {
		return nil, bcAddr, fmt.Errorf("bonding curve account not found")
	}

	raw := res.Value[0].Data.GetBinary()
	if len(raw) < 16 {
		return nil, bcAddr, fmt.Errorf("invalid bonding curve data length")
	}

	bc := &BondingCurve{
		VirtualTokenReserves: binary.LittleEndian.Uint64(raw[:8]),
		VirtualSolReserves:   binary.LittleEndian.Uint64(raw[8:16]),
	}

	d.bcCache.mu.Lock()
	d.bcCache.data = bc
	d.bcCache.fetchedAt = time.Now()
	d.bcCache.mu.Unlock()

	return bc, bcAddr, nil
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
