// internal/blockchain/solbc/transaction/types.go
package transaction

import (
	"errors"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
)

var (
	ErrConfirmationTimeout = errors.New("transaction confirmation timeout")
	ErrInvalidSignature    = errors.New("invalid transaction signature")
	ErrInvalidBlockhash    = errors.New("invalid blockhash")
	ErrInvalidInstruction  = errors.New("invalid instruction")
)

type Config struct {
	MaxRetries       int
	RetryDelay       time.Duration
	ConfirmationTime time.Duration
	PriorityFee      uint64
	ComputeUnits     uint32
	SkipPreflight    bool
	Commitment       rpc.CommitmentType
	MinConfirmations uint8
}

type Status struct {
	Signature     string
	Status        string
	Confirmations uint64
	Slot          uint64
	Error         string
	Timestamp     time.Time
	BlockTime     *time.Time
}
