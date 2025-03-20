// =============================
// File: internal/dex/pumpfun/types.go
// =============================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
)

// GlobalAccount represents the structure of the PumpFun global account data
type GlobalAccount struct {
	Discriminator  [8]byte
	Initialized    bool
	Authority      solana.PublicKey
	FeeRecipient   solana.PublicKey
	FeeBasisPoints uint64
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
