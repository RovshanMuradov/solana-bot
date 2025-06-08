// internal/protocol/types.go
package protocol

import (
	"context"
)

// Type represents the type of protocol.
type Type string

const (
	TypeDEX     Type = "dex"
	TypeNFT     Type = "nft"
	TypeStaking Type = "staking"
	TypeLending Type = "lending"
)

// AssetType represents the type of asset.
type AssetType string

const (
	AssetToken AssetType = "token"
	AssetNFT   AssetType = "nft"
	AssetLP    AssetType = "lp"
)

// Capability represents a protocol capability.
type Capability string

const (
	CapabilitySwap     Capability = "swap"
	CapabilityMint     Capability = "mint"
	CapabilityBurn     Capability = "burn"
	CapabilityStake    Capability = "stake"
	CapabilityUnstake  Capability = "unstake"
	CapabilityBuy      Capability = "buy"
	CapabilitySell     Capability = "sell"
	CapabilityTransfer Capability = "transfer"
)

// Protocol is the main interface for all blockchain protocols.
type Protocol interface {
	// GetName returns the protocol name.
	GetName() string

	// GetType returns the protocol type.
	GetType() Type

	// GetCapabilities returns supported capabilities.
	GetCapabilities() []Capability

	// SupportsAsset checks if the protocol supports a specific asset type.
	SupportsAsset(assetType AssetType) bool

	// CreateOperation creates an operation for execution.
	CreateOperation(ctx context.Context, params OperationParams) (Operation, error)

	// GetAssetInfo retrieves information about an asset.
	GetAssetInfo(ctx context.Context, assetID string) (*AssetInfo, error)

	// HealthCheck verifies protocol connectivity and status.
	HealthCheck(ctx context.Context) error
}

// OperationParams contains parameters for creating an operation.
type OperationParams struct {
	Type      string                 // Operation type (e.g., "swap", "buy", "sell")
	AssetType AssetType              // Type of asset
	AssetID   string                 // Asset identifier (mint address, NFT ID, etc.)
	Amount    float64                // Amount for the operation
	Metadata  map[string]interface{} // Protocol-specific parameters
}

// Operation represents an executable operation.
type Operation interface {
	// Execute runs the operation.
	Execute(ctx context.Context) (*OperationResult, error)

	// Validate checks if the operation can be executed.
	Validate(ctx context.Context) error

	// EstimateFees returns estimated fees for the operation.
	EstimateFees(ctx context.Context) (*FeeEstimate, error)

	// GetParams returns the operation parameters.
	GetParams() OperationParams
}

// OperationResult contains the result of an operation.
type OperationResult struct {
	Success       bool
	TransactionID string
	AssetReceived *AssetAmount
	AssetSent     *AssetAmount
	Fees          *FeeBreakdown
	Metadata      map[string]interface{}
	Error         error
}

// AssetInfo contains information about an asset.
type AssetInfo struct {
	ID       string
	Type     AssetType
	Name     string
	Symbol   string
	Decimals uint8
	Supply   uint64
	Price    float64
	Metadata map[string]interface{}
}

// AssetAmount represents an amount of an asset.
type AssetAmount struct {
	AssetID  string
	Amount   uint64
	Decimals uint8
}

// FeeEstimate contains estimated fees.
type FeeEstimate struct {
	NetworkFee  uint64
	ProtocolFee uint64
	TotalFee    uint64
}

// FeeBreakdown contains actual fees paid.
type FeeBreakdown struct {
	NetworkFee  uint64
	ProtocolFee uint64
	OtherFees   map[string]uint64
	TotalFee    uint64
}
