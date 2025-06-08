// internal/protocol/adapter.go
package protocol

import (
	"context"
	"fmt"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

// DEXAdapter wraps a DEX implementation to provide Protocol interface.
type DEXAdapter struct {
	dex    dex.DEX
	logger *zap.Logger
}

// NewDEXAdapter creates a new adapter for a DEX.
func NewDEXAdapter(d dex.DEX, logger *zap.Logger) Protocol {
	return &DEXAdapter{
		dex:    d,
		logger: logger.Named("dex_adapter"),
	}
}

// GetName returns the protocol name.
func (a *DEXAdapter) GetName() string {
	return a.dex.GetName()
}

// GetType returns the protocol type.
func (a *DEXAdapter) GetType() Type {
	return TypeDEX
}

// GetCapabilities returns supported capabilities.
func (a *DEXAdapter) GetCapabilities() []Capability {
	// DEXes typically support swap, buy, and sell
	return []Capability{
		CapabilitySwap,
		CapabilityBuy,
		CapabilitySell,
	}
}

// SupportsAsset checks if the protocol supports a specific asset type.
func (a *DEXAdapter) SupportsAsset(assetType AssetType) bool {
	// DEXes typically only support tokens
	return assetType == AssetToken
}

// CreateOperation creates an operation for execution.
func (a *DEXAdapter) CreateOperation(ctx context.Context, params OperationParams) (Operation, error) {
	// Convert protocol operation params to DEX task
	t := &task.Task{
		TokenMint: params.AssetID,
		AmountSol: params.Amount,
	}

	// Extract DEX-specific parameters from metadata
	if slippage, ok := params.Metadata["slippage"].(float64); ok {
		t.SlippagePercent = slippage
	}
	if priority, ok := params.Metadata["priority_fee"].(string); ok {
		t.PriorityFeeSol = priority
	}
	if computeUnits, ok := params.Metadata["compute_units"].(uint32); ok {
		t.ComputeUnits = computeUnits
	}

	// Map operation type to task operation
	switch params.Type {
	case "buy", "snipe":
		t.Operation = task.OperationSnipe
	case "swap":
		t.Operation = task.OperationSwap
	case "sell":
		t.Operation = task.OperationSell
	default:
		return nil, fmt.Errorf("unsupported operation type: %s", params.Type)
	}

	return &dexOperation{
		dex:    a.dex,
		task:   t,
		params: params,
		logger: a.logger,
	}, nil
}

// GetAssetInfo retrieves information about an asset.
func (a *DEXAdapter) GetAssetInfo(ctx context.Context, assetID string) (*AssetInfo, error) {
	// For DEX, we can get token balance and price
	balance, err := a.dex.GetTokenBalance(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance: %w", err)
	}

	// Price calculation would require bonding curve info
	// For now, set to 0 as this is a placeholder
	price := 0.0

	return &AssetInfo{
		ID:       assetID,
		Type:     AssetToken,
		Supply:   balance,
		Price:    price,
		Decimals: 6, // Default for Solana tokens
		Metadata: map[string]interface{}{
			"dex": a.dex.GetName(),
		},
	}, nil
}

// HealthCheck verifies protocol connectivity and status.
func (a *DEXAdapter) HealthCheck(ctx context.Context) error {
	// Try to get a known token balance as a health check
	_, err := a.dex.GetTokenBalance(ctx, "So11111111111111111111111111111111111111112") // WSOL
	return err
}

// dexOperation implements Operation interface for DEX operations.
type dexOperation struct {
	dex    dex.DEX
	task   *task.Task
	params OperationParams
	logger *zap.Logger
}

// Execute runs the operation.
func (o *dexOperation) Execute(ctx context.Context) (*OperationResult, error) {
	// Execute through DEX
	err := o.dex.Execute(ctx, o.task)
	if err != nil {
		return &OperationResult{
			Success: false,
			Error:   err,
		}, err
	}

	// For now, return a basic success result
	// In a real implementation, we would parse transaction results
	return &OperationResult{
		Success: true,
		Metadata: map[string]interface{}{
			"dex":  o.dex.GetName(),
			"task": o.task.TaskName,
		},
	}, nil
}

// Validate checks if the operation can be executed.
func (o *dexOperation) Validate(_ context.Context) error {
	// Validate task parameters
	return o.task.Validate()
}

// EstimateFees returns estimated fees for the operation.
func (o *dexOperation) EstimateFees(_ context.Context) (*FeeEstimate, error) {
	// Basic fee estimation
	// In a real implementation, this would calculate actual fees
	return &FeeEstimate{
		NetworkFee:  5000, // 0.000005 SOL
		ProtocolFee: 0,    // Depends on DEX
		TotalFee:    5000,
	}, nil
}

// GetParams returns the operation parameters.
func (o *dexOperation) GetParams() OperationParams {
	return o.params
}
