// internal/dex/raydium/constants.go
package raydium

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// Program IDs
var (
	// Используем MPK для краткости, так как это константы
	TokenProgramID     = solana.MPK("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	RaydiumV4ProgramID = solana.MPK("675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8")
	SystemProgramID    = solana.MPK("11111111111111111111111111111111")
	SysvarRentPubkey   = solana.MPK("SysvarRent111111111111111111111111111111111")
	WrappedSolMint     = solana.MPK("So11111111111111111111111111111111111111112")
)

// Compute budget constants
const (
	MaxComputeUnitLimit = 300000
	DefaultComputePrice = 1000
	MinComputePrice     = 0
	MaxComputePrice     = 100000
)

// Pool account layout constants
const (
	PoolAccountSize     = 388
	PoolVersionOffset   = 0
	StatusOffset        = 1
	BaseMintOffset      = 8
	QuoteMintOffset     = 40
	LpMintOffset        = 72
	BaseVaultOffset     = 104
	QuoteVaultOffset    = 136
	DecimalsOffset      = 168
	FeeBpsOffset        = 170
	PoolStatusOffset    = 188
	AmmOpenOrders       = 196
	MarketIDOffset      = 228
	TargetOrdersOffset  = 260
	WithdrawQueueOffset = 292
)

// Pool status
const (
	PoolStatusUninitialized uint8 = 0
	PoolStatusInitialized   uint8 = 1
	PoolStatusDisabled      uint8 = 2
	PoolStatusActive        uint8 = 3
)

// PDA seeds
const (
	AmmAuthorityLayout = "amm_authority"
	PoolTempLpLayout   = "pool_temp_lp"
	PoolWithdrawQueue  = "withdraw_queue"
	TargetOrdersSeed   = "target_orders"
	OpenOrdersSeed     = "open_orders"
)

// Swap constants
const (
	DefaultSlippagePercent = 0.5
	MaxSlippagePercent     = 5.0
	MinSwapAmount          = 1000
	MaxTokensInPool        = 100000
	TradeDirectionIn       = "in"
	TradeDirectionOut      = "out"
)

// Error codes
const (
	ErrPoolNotFound      = "POOL_NOT_FOUND"
	ErrInvalidPoolStatus = "INVALID_POOL_STATUS"
	ErrInsufficientFunds = "INSUFFICIENT_FUNDS"
	ErrSlippageExceeded  = "SLIPPAGE_EXCEEDED"
	ErrInvalidMint       = "INVALID_MINT"
	ErrInvalidAmount     = "INVALID_AMOUNT"
	ErrPoolDisabled      = "POOL_DISABLED"
	ErrInvalidDirection  = "INVALID_DIRECTION"
)

// Account size constants
const (
	TokenAccountSize = 165
	MintAccountSize  = 82
)

// Также добавим константы для версий пула
const (
	MinPoolDecimals uint8 = 0
	MaxPoolDecimals uint8 = 255
)

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Добавляем конструктор для RaydiumError
func NewRaydiumError(code string, message string, details map[string]interface{}) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Details: details,
	}
}
