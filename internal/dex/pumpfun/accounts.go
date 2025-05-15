// =============================
// File: internal/dex/pumpfun/accounts.go
// =============================
package pumpfun

import (
	"context"
	"encoding/binary"
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
	// 1) Деривация PDA bondingCurve
	bcAddr, _, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, solana.PublicKey{}, err
	}

	// 2) Попытка взять из кэша
	d.bcCache.mu.RLock()
	if d.bcCache.data != nil && time.Since(d.bcCache.fetchedAt) < bcCacheTTL {
		data := d.bcCache.data
		d.bcCache.mu.RUnlock()
		return data, bcAddr, nil
	}
	d.bcCache.mu.RUnlock()

	// 3) Запрашиваем с цепочки
	res, err := d.client.GetMultipleAccounts(ctx, []solana.PublicKey{bcAddr})
	if err != nil {
		return nil, bcAddr, err
	}
	if len(res.Value) == 0 || res.Value[0] == nil {
		return nil, bcAddr, fmt.Errorf("bonding curve account not found")
	}

	raw := res.Value[0].Data.GetBinary()

	// Проверяем, что у нас достаточно данных для дискриминатора и базовых полей
	// 8 (дискриминатор) + 8*5 (u64*5) + 1 (bool) = 49 байт минимум
	if len(raw) < 49 {
		return nil, bcAddr, fmt.Errorf("bonding curve data too short for basic fields: %d bytes", len(raw))
	}

	// Пропускаем первые 8 байт (дискриминатор)
	dataWithoutDiscriminator := raw[8:]

	// 4) Десериализация полей (без дискриминатора)
	bc := &BondingCurve{
		VirtualTokenReserves: binary.LittleEndian.Uint64(dataWithoutDiscriminator[0:8]),
		VirtualSolReserves:   binary.LittleEndian.Uint64(dataWithoutDiscriminator[8:16]),
		RealTokenReserves:    binary.LittleEndian.Uint64(dataWithoutDiscriminator[16:24]),
		RealSolReserves:      binary.LittleEndian.Uint64(dataWithoutDiscriminator[24:32]),
		TokenTotalSupply:     binary.LittleEndian.Uint64(dataWithoutDiscriminator[32:40]),
		Complete:             dataWithoutDiscriminator[40] != 0,
	}

	// Проверяем, есть ли данные для поля Creator (должно быть минимум 40+1+32 байт)
	if len(dataWithoutDiscriminator) >= 41+32 {
		bc.Creator = solana.PublicKeyFromBytes(dataWithoutDiscriminator[41:73])
		d.logger.Debug("Parsed creator from bonding curve",
			zap.String("creator", bc.Creator.String()),
			zap.String("bonding_curve", bcAddr.String()))
	} else {
		// Если данных недостаточно, устанавливаем Creator в пустой PublicKey
		bc.Creator = solana.PublicKey{}
		d.logger.Warn("Bonding curve data too short to include Creator field",
			zap.Int("data_length", len(dataWithoutDiscriminator)),
			zap.String("bonding_curve", bcAddr.String()))
	}

	// 5) Обновляем кэш
	d.bcCache.mu.Lock()
	d.bcCache.data = bc
	d.bcCache.fetchedAt = time.Now()
	d.bcCache.mu.Unlock()

	return bc, bcAddr, nil
}

// DeriveCreatorVaultPDA определяет адрес creator-vault PDA на основе адреса создателя токена
// Реализация соответствует Python-коду для поиска creator-vault
func DeriveCreatorVaultPDA(programID, creator solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress(
		[][]byte{[]byte("creator-vault"), creator.Bytes()},
		programID,
	)
}

// FetchGlobalAccount получает и парсит данные глобального аккаунта Pump.fun.
func FetchGlobalAccount(ctx context.Context, client *solbc.Client, globalAddr solana.PublicKey, logger *zap.Logger) (*GlobalAccount, error) {
	// Получение информации об аккаунте с блокчейна
	start := time.Now()
	accountInfo, err := client.GetAccountInfo(ctx, globalAddr)

	if logger != nil {
		logger.Debug("RPC:GetAccountInfo",
			zap.String("account", globalAddr.String()),
			zap.Duration("duration", time.Since(start)))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get global account: %w", err)
	}

	// Проверка существования аккаунта
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("global account not found: %s", globalAddr.String())
	}

	// Проверка владельца аккаунта
	if !accountInfo.Value.Owner.Equals(PumpFunProgramID) {
		return nil, fmt.Errorf("global account has incorrect owner: expected %s, got %s",
			PumpFunProgramID.String(), accountInfo.Value.Owner.String())
	}

	// Извлечение бинарных данных из аккаунта
	data := accountInfo.Value.Data.GetBinary()

	// Проверка достаточности данных
	minDataLen := 81 // 8 (дискриминатор) + 1 (флаг) + 32 (authority) + 32 (feeRecipient) + 8 (feeBasisPoints)
	if len(data) < minDataLen {
		return nil, fmt.Errorf("global account data too short: %d bytes", len(data))
	}

	// Десериализация данных
	account := &GlobalAccount{}

	// Базовые поля
	copy(account.Discriminator[:], data[0:8])
	account.Initialized = data[8] != 0
	account.Authority = solana.PublicKeyFromBytes(data[9:41])
	account.FeeRecipient = solana.PublicKeyFromBytes(data[41:73])

	// Расширенные поля - обрабатываем более структурированно
	offset := 73

	// InitialVirtualTokenRes, InitialVirtualSolRes, InitialRealTokenRes, TokenTotalSupply
	if len(data) >= offset+32 {
		account.InitialVirtualTokenRes = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
		account.InitialVirtualSolRes = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
		account.InitialRealTokenRes = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
		account.TokenTotalSupply = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	} else {
		return nil, fmt.Errorf("global account data missing initial reserves fields")
	}

	// FeeBasisPoints
	if len(data) >= offset+8 {
		account.FeeBasisPoints = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	} else {
		return nil, fmt.Errorf("global account data missing fee basis points")
	}

	// WithdrawAuthority
	if len(data) >= offset+32 {
		account.WithdrawAuthority = solana.PublicKeyFromBytes(data[offset : offset+32])
		offset += 32
	} else {
		return nil, fmt.Errorf("global account data missing withdraw authority")
	}

	// EnableMigrate
	if len(data) >= offset+1 {
		account.EnableMigrate = data[offset] != 0
		offset++
	} else {
		return nil, fmt.Errorf("global account data missing enable migrate flag")
	}

	// PoolMigrationFee
	if len(data) >= offset+8 {
		account.PoolMigrationFee = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	} else {
		return nil, fmt.Errorf("global account data missing pool migration fee")
	}

	// CreatorFeeBasisPoints
	if len(data) >= offset+8 {
		account.CreatorFeeBasisPoints = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8

		// Парсинг FeeRecipients (7 адресов по 32 байта)
		if len(data) >= offset+7*32 {
			for i := 0; i < 7; i++ {
				account.FeeRecipients[i] = solana.PublicKeyFromBytes(data[offset : offset+32])
				offset += 32
			}
		}

		// SetCreatorAuthority
		if len(data) >= offset+32 {
			account.SetCreatorAuthority = solana.PublicKeyFromBytes(data[offset : offset+32])
		}
	}

	return account, nil
}
