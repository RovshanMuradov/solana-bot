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
	const minLen = 8*5 + 1 + 32 // 5×u64 + bool + Pubkey
	if len(raw) < minLen {
		return nil, bcAddr, fmt.Errorf("bonding curve data too short: %d bytes", len(raw))
	}

	// 4) Десериализация полей
	bc := &BondingCurve{
		VirtualTokenReserves: binary.LittleEndian.Uint64(raw[0:8]),
		VirtualSolReserves:   binary.LittleEndian.Uint64(raw[8:16]),
		RealTokenReserves:    binary.LittleEndian.Uint64(raw[16:24]),
		RealSolReserves:      binary.LittleEndian.Uint64(raw[24:32]),
		TokenTotalSupply:     binary.LittleEndian.Uint64(raw[32:40]),
		Complete:             raw[40] != 0,
		Creator:              solana.PublicKeyFromBytes(raw[41:73]),
	}

	// 5) Обновляем кэш
	d.bcCache.mu.Lock()
	d.bcCache.data = bc
	d.bcCache.fetchedAt = time.Now()
	d.bcCache.mu.Unlock()

	return bc, bcAddr, nil
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
	account.FeeBasisPoints = binary.LittleEndian.Uint64(data[73:81])

	// Расширенные поля (если достаточно данных)
	offset := 81

	// Проверяем доступность дополнительных полей перед чтением
	if len(data) >= offset+32 {
		account.WithdrawAuthority = solana.PublicKeyFromBytes(data[offset : offset+32])
		offset += 32

		if len(data) >= offset+1 {
			account.EnableMigrate = data[offset] != 0
			offset++

			if len(data) >= offset+8 {
				account.PoolMigrationFee = binary.LittleEndian.Uint64(data[offset : offset+8])
				offset += 8

				if len(data) >= offset+8 {
					account.CreatorFeeBasisPoints = binary.LittleEndian.Uint64(data[offset : offset+8])
					// Дальнейший парсинг fee_recipients и set_creator_authority можно добавить по необходимости
				}
			}
		}
	}

	return account, nil
}
