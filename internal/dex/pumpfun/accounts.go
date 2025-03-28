// =============================
// File: internal/dex/pumpfun/accounts.go
// =============================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"math"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// deriveBondingCurveAccounts выводит адреса bonding curve и ассоциированного токен-аккаунта
func (d *DEX) deriveBondingCurveAccounts(ctx context.Context) (bondingCurve, associatedBondingCurve solana.PublicKey, err error) {
	bondingCurve, _, err = solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to derive bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve", zap.String("address", bondingCurve.String()))

	associatedBondingCurve, _, err = solana.FindAssociatedTokenAddress(bondingCurve, d.config.Mint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}
	d.logger.Debug("Derived bonding curve ATA", zap.String("address", associatedBondingCurve.String()))

	return bondingCurve, associatedBondingCurve, nil
}

// FetchBondingCurveAccount получает и парсит данные аккаунта bonding curve
func (d *DEX) FetchBondingCurveAccount(ctx context.Context, bondingCurve solana.PublicKey) (*BondingCurve, error) {
	accountInfo, err := d.client.GetAccountInfo(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve account: %w", err)
	}

	if accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}

	data := accountInfo.Value.Data.GetBinary()
	if len(data) < 16 {
		return nil, fmt.Errorf("invalid bonding curve data: insufficient length")
	}

	virtualTokenReserves := binary.LittleEndian.Uint64(data[0:8])
	virtualSolReserves := binary.LittleEndian.Uint64(data[8:16])

	return &BondingCurve{
		VirtualTokenReserves: virtualTokenReserves,
		VirtualSolReserves:   virtualSolReserves,
	}, nil
}

// FetchGlobalAccount fetches and parses the global account data
func FetchGlobalAccount(ctx context.Context, client *solbc.Client, globalAddr solana.PublicKey) (*GlobalAccount, error) {
	// Get account info from the blockchain
	accountInfo, err := client.GetAccountInfo(ctx, globalAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get global account: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("global account not found: %s", globalAddr.String())
	}

	// Make sure the account is owned by the PumpFun program
	if !accountInfo.Value.Owner.Equals(PumpFunProgramID) {
		return nil, fmt.Errorf("global account has incorrect owner: expected %s, got %s",
			PumpFunProgramID.String(), accountInfo.Value.Owner.String())
	}

	// Get binary data
	data := accountInfo.Value.Data.GetBinary()

	// Need at least the discriminator + initialized flag + two public keys (32 bytes each)
	if len(data) < 8+1+64 {
		return nil, fmt.Errorf("global account data too short: %d bytes", len(data))
	}

	// Deserialize the data
	account := &GlobalAccount{}

	// Read discriminator (8 bytes)
	copy(account.Discriminator[:], data[0:8])

	// Read initialized flag (1 byte)
	account.Initialized = data[8] != 0

	// Read authority public key (32 bytes)
	authorityBytes := make([]byte, 32)
	copy(authorityBytes, data[9:41])
	account.Authority = solana.PublicKeyFromBytes(authorityBytes)

	// Read fee recipient public key (32 bytes)
	feeRecipientBytes := make([]byte, 32)
	copy(feeRecipientBytes, data[41:73])
	account.FeeRecipient = solana.PublicKeyFromBytes(feeRecipientBytes)

	// Read fee basis points (8 bytes)
	if len(data) >= 81 {
		account.FeeBasisPoints = binary.LittleEndian.Uint64(data[73:81])
	}

	return account, nil
}

// GetTokenPrice возвращает текущую цену токена на основе bonding curve
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if d.config.Mint.String() != tokenMint {
		return 0, fmt.Errorf("token %s not configured in this DEX instance", tokenMint)
	}

	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to derive bonding curve: %w", err)
	}

	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch bonding curve data: %w", err)
	}

	price := float64(bondingCurveData.VirtualSolReserves) / float64(bondingCurveData.VirtualTokenReserves)
	return math.Floor(price*1e9) / 1e9, nil
}
