package pumpswap

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// Account discriminators extracted from the IDL
var (
	// GlobalConfigDiscriminator is the discriminator for GlobalConfig accounts
	GlobalConfigDiscriminator = []byte{149, 8, 156, 202, 160, 252, 176, 217}

	// PoolDiscriminator is the discriminator for Pool accounts
	PoolDiscriminator = []byte{241, 154, 109, 4, 17, 177, 109, 188}
)

// GlobalConfig represents the global configuration for PumpSwap
type GlobalConfig struct {
	Admin                  solana.PublicKey    // The admin public key
	LPFeeBasisPoints       uint64              // LP fee in basis points (0.01%)
	ProtocolFeeBasisPoints uint64              // Protocol fee in basis points (0.01%)
	DisableFlags           uint8               // Flags to disable certain functionality
	ProtocolFeeRecipients  [8]solana.PublicKey // Addresses of protocol fee recipients
}

// DisableFlags bits in GlobalConfig
const (
	DisableCreatePool = 1 << iota
	DisableDeposit
	DisableWithdraw
	DisableBuy
	DisableSell
)

// Pool represents a liquidity pool in PumpSwap
type Pool struct {
	PoolBump              uint8            // PDA bump
	Index                 uint16           // Pool index
	Creator               solana.PublicKey // Creator of the pool
	BaseMint              solana.PublicKey // Base token mint (usually SOL)
	QuoteMint             solana.PublicKey // Quote token mint
	LPMint                solana.PublicKey // LP token mint
	PoolBaseTokenAccount  solana.PublicKey // Pool's base token account
	PoolQuoteTokenAccount solana.PublicKey // Pool's quote token account
	LPSupply              uint64           // True circulating supply of LP tokens
}

// PoolInfo contains information about the state of a liquidity pool
type PoolInfo struct {
	Address               solana.PublicKey // Pool address
	BaseMint              solana.PublicKey // Base token mint
	QuoteMint             solana.PublicKey // Quote token mint
	BaseReserves          uint64           // Amount of base tokens in the pool
	QuoteReserves         uint64           // Amount of quote tokens in the pool
	LPSupply              uint64           // LP token supply
	FeesBasisPoints       uint64           // LP fee in basis points
	ProtocolFeeBPS        uint64           // Protocol fee in basis points
	LPMint                solana.PublicKey // LP token mint
	PoolBaseTokenAccount  solana.PublicKey // Pool's base token account
	PoolQuoteTokenAccount solana.PublicKey // Pool's quote token account
}

// ParseGlobalConfig parses account data into GlobalConfig structure
func ParseGlobalConfig(data []byte) (*GlobalConfig, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for GlobalConfig")
	}

	// Verify discriminator
	for i := 0; i < 8; i++ {
		if data[i] != GlobalConfigDiscriminator[i] {
			return nil, fmt.Errorf("invalid discriminator for GlobalConfig")
		}
	}

	// Adjust the position for the data after discriminator
	pos := 8

	// Ensure enough data is available
	if len(data) < pos+32+8+8+1+(32*8) {
		return nil, fmt.Errorf("data too short for GlobalConfig content")
	}

	config := &GlobalConfig{}

	// Parse the Admin field (pubkey - 32 bytes)
	adminBytes := make([]byte, 32)
	copy(adminBytes, data[pos:pos+32])
	config.Admin = solana.PublicKeyFromBytes(adminBytes)
	pos += 32

	// Parse the LP fee basis points (u64 - 8 bytes)
	config.LPFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	// Parse the protocol fee basis points (u64 - 8 bytes)
	config.ProtocolFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	// Parse disable flags (u8 - 1 byte)
	config.DisableFlags = data[pos]
	pos++

	// Parse protocol fee recipients (array of 8 pubkeys - 32*8 bytes)
	for i := 0; i < 8; i++ {
		recipientBytes := make([]byte, 32)
		copy(recipientBytes, data[pos:pos+32])
		config.ProtocolFeeRecipients[i] = solana.PublicKeyFromBytes(recipientBytes)
		pos += 32
	}

	return config, nil
}

// ParsePool parses account data into Pool structure
func ParsePool(data []byte) (*Pool, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for Pool")
	}

	// Verify discriminator
	for i := 0; i < 8; i++ {
		if data[i] != PoolDiscriminator[i] {
			return nil, fmt.Errorf("invalid discriminator for Pool")
		}
	}

	// Adjust the position for the data after discriminator
	pos := 8

	// Ensure enough data is available
	if len(data) < pos+1+2+32+32+32+32+32+32+8 {
		return nil, fmt.Errorf("data too short for Pool content")
	}

	pool := &Pool{}

	// Parse pool bump (u8 - 1 byte)
	pool.PoolBump = data[pos]
	pos++

	// Parse index (u16 - 2 bytes)
	pool.Index = uint16(data[pos]) | (uint16(data[pos+1]) << 8)
	pos += 2

	// Parse creator public key (pubkey - 32 bytes)
	creatorBytes := make([]byte, 32)
	copy(creatorBytes, data[pos:pos+32])
	pool.Creator = solana.PublicKeyFromBytes(creatorBytes)
	pos += 32

	// Parse base mint (pubkey - 32 bytes)
	baseMintBytes := make([]byte, 32)
	copy(baseMintBytes, data[pos:pos+32])
	pool.BaseMint = solana.PublicKeyFromBytes(baseMintBytes)
	pos += 32

	// Parse quote mint (pubkey - 32 bytes)
	quoteMintBytes := make([]byte, 32)
	copy(quoteMintBytes, data[pos:pos+32])
	pool.QuoteMint = solana.PublicKeyFromBytes(quoteMintBytes)
	pos += 32

	// Parse LP mint (pubkey - 32 bytes)
	lpMintBytes := make([]byte, 32)
	copy(lpMintBytes, data[pos:pos+32])
	pool.LPMint = solana.PublicKeyFromBytes(lpMintBytes)
	pos += 32

	// Parse pool base token account (pubkey - 32 bytes)
	poolBaseTokenAccountBytes := make([]byte, 32)
	copy(poolBaseTokenAccountBytes, data[pos:pos+32])
	pool.PoolBaseTokenAccount = solana.PublicKeyFromBytes(poolBaseTokenAccountBytes)
	pos += 32

	// Parse pool quote token account (pubkey - 32 bytes)
	poolQuoteTokenAccountBytes := make([]byte, 32)
	copy(poolQuoteTokenAccountBytes, data[pos:pos+32])
	pool.PoolQuoteTokenAccount = solana.PublicKeyFromBytes(poolQuoteTokenAccountBytes)
	pos += 32

	// Parse LP supply (u64 - 8 bytes)
	pool.LPSupply = binary.LittleEndian.Uint64(data[pos : pos+8])

	return pool, nil
}
