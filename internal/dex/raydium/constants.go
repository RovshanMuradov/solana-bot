// internal/dex/raydium/constants.go
package raydium

import "fmt"

// Program IDs
const (
	RAYDIUM_V4_PROGRAM_ID = "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8"
	TOKEN_PROGRAM_ID      = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	SYSTEM_PROGRAM_ID     = "11111111111111111111111111111111"
	SYSVAR_RENT_PUBKEY    = "SysvarRent111111111111111111111111111111111"
	WRAPPED_SOL_MINT      = "So11111111111111111111111111111111111111112" // Добавлено
)

// Compute budget constants
const (
	MAX_COMPUTE_UNIT_LIMIT = 300000
	DEFAULT_COMPUTE_PRICE  = 1000
	MIN_COMPUTE_PRICE      = 0      // Добавлено
	MAX_COMPUTE_PRICE      = 100000 // Добавлено
)

// Pool account layout constants
const (
	POOL_ACCOUNT_SIZE     = 388
	POOL_VERSION_OFFSET   = 0 // Добавлено
	STATUS_OFFSET         = 1 // Добавлено
	BASE_MINT_OFFSET      = 8
	QUOTE_MINT_OFFSET     = 40
	LP_MINT_OFFSET        = 72 // Добавлено
	BASE_VAULT_OFFSET     = 104
	QUOTE_VAULT_OFFSET    = 136
	DECIMALS_OFFSET       = 168
	FEE_BPS_OFFSET        = 170
	POOL_STATUS_OFFSET    = 188
	AMM_OPEN_ORDERS       = 196
	MARKET_ID_OFFSET      = 228
	TARGET_ORDERS_OFFSET  = 260 // Добавлено
	WITHDRAW_QUEUE_OFFSET = 292 // Добавлено
)

// Pool status
const (
	POOL_STATUS_UNINITIALIZED uint8 = 0
	POOL_STATUS_INITIALIZED   uint8 = 1
	POOL_STATUS_DISABLED      uint8 = 2
	POOL_STATUS_ACTIVE        uint8 = 3
)

// PDA seeds
const (
	AMM_AUTHORITY_LAYOUT = "amm_authority"
	POOL_TEMP_LP_LAYOUT  = "pool_temp_lp"
	POOL_WITHDRAW_QUEUE  = "withdraw_queue"
	TARGET_ORDERS_SEED   = "target_orders" // Добавлено
	OPEN_ORDERS_SEED     = "open_orders"   // Добавлено
)

// Swap constants
const (
	DEFAULT_SLIPPAGE_PERCENT = 0.5
	MAX_SLIPPAGE_PERCENT     = 5.0
	MIN_SWAP_AMOUNT          = 1000   // Добавлено: минимальная сумма для свапа в лампортах
	MAX_TOKENS_IN_POOL       = 100000 // Добавлено: максимальное количество токенов в пуле
	TRADE_DIRECTION_IN       = "in"   // Добавлено
	TRADE_DIRECTION_OUT      = "out"  // Добавлено
)

// Error codes
const (
	ERR_POOL_NOT_FOUND      = "POOL_NOT_FOUND" // Изменено на uppercase
	ERR_INVALID_POOL_STATUS = "INVALID_POOL_STATUS"
	ERR_INSUFFICIENT_FUNDS  = "INSUFFICIENT_FUNDS"
	ERR_SLIPPAGE_EXCEEDED   = "SLIPPAGE_EXCEEDED"
	ERR_INVALID_MINT        = "INVALID_MINT"
	ERR_INVALID_AMOUNT      = "INVALID_AMOUNT"    // Добавлено
	ERR_POOL_DISABLED       = "POOL_DISABLED"     // Добавлено
	ERR_INVALID_DIRECTION   = "INVALID_DIRECTION" // Добавлено
)

// Account size constants
const (
	TOKEN_ACCOUNT_SIZE = 165 // Добавлено
	MINT_ACCOUNT_SIZE  = 82  // Добавлено
)

// RaydiumError represents a custom error type
type RaydiumError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *RaydiumError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Добавляем конструктор для RaydiumError
func NewRaydiumError(code string, message string, details map[string]interface{}) *RaydiumError {
	return &RaydiumError{
		Code:    code,
		Message: message,
		Details: details,
	}
}
