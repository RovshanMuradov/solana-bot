// =============================================
// File: internal/dex/pumpfun/global_account.go
// =============================================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// FetchGlobalAccount fetches and deserializes the global account data
func FetchGlobalAccount(ctx context.Context, client *solbc.Client, globalAddr solana.PublicKey, logger *zap.Logger) (*GlobalAccount, error) {
	// Get account info from the blockchain
	accountInfo, err := client.GetAccountInfo(ctx, globalAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get global account: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("global account not found: %s", globalAddr.String())
	}

	// Make sure the account is owned by the PumpFun program
	programID := PumpFunProgramID

	if !accountInfo.Value.Owner.Equals(programID) {
		return nil, fmt.Errorf("global account has incorrect owner: expected %s, got %s",
			programID.String(), accountInfo.Value.Owner.String())
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

	// Read uint64 values (each 8 bytes)
	offset := 73
	account.InitialVirtualTokenReserves = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	account.InitialVirtualSolReserves = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	account.InitialRealTokenReserves = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	account.TokenTotalSupply = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	account.FeeBasisPoints = binary.LittleEndian.Uint64(data[offset : offset+8])

	logger.Info("Global account data parsed successfully",
		zap.String("fee_recipient", account.FeeRecipient.String()))

	return account, nil
}
