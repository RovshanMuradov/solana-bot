# Refactoring Changes Documentation

## Overview
This document describes the changes made during the Phase 1 refactoring to prepare the codebase for future extensibility. The changes follow the plan outlined in TODO.md and introduce foundational components for supporting multiple protocols beyond DEX trading.

## New Components Added

### 1. Event Bus System (`internal/events/`)
**Purpose**: Decouples components by providing asynchronous event-driven communication.

**Files added**:
- `bus.go` - Core event bus implementation with pub/sub pattern
- `types.go` - Event type definitions (OperationStarted, PriceUpdated, etc.)
- `handler.go` - Event handler interfaces

**Key features**:
- Non-blocking event publishing
- Buffered channels to prevent blocking
- Graceful shutdown support
- Event statistics tracking

### 2. Protocol Abstraction (`internal/protocol/`)
**Purpose**: Generic interface for supporting different blockchain protocols (DEX, NFT, Staking, etc.)

**Files added**:
- `types.go` - Protocol interface and related types
- `adapter.go` - DEXAdapter to wrap existing DEX implementations
- `registry.go` - Central registry for protocol management

**Key interfaces**:
```go
type Protocol interface {
    GetCapabilities() []Capability
    CreateOperation(ctx, params) (Operation, error)
    GetAssetInfo(ctx, assetID) (*AssetInfo, error)
}
```

### 3. Enhanced Task Structure
**Purpose**: Support for multiple protocols and asset types beyond tokens.

**Changes to `internal/task/task.go`**:
- Added `Protocol` field (defaults to "dex" for backward compatibility)
- Added `AssetType` field (defaults to "token")
- Added `Metadata` map for extensible parameters
- New helper methods: `GetProtocol()`, `GetAssetType()`, `GetMetadata()`

## Integration Points

### 1. Event Publishing Integration
**Files modified**:
- `internal/bot/runner.go` - Event bus initialization and debug logging
- `internal/bot/worker.go` - Publishes operation lifecycle events
- `internal/bot/worker_monitor.go` - Publishes price updates and monitoring events

**Events published**:
- `OperationStarted` - When a task execution begins
- `OperationCompleted` - When a task succeeds
- `OperationFailed` - When a task fails
- `PriceUpdated` - On each price check during monitoring
- `MonitoringStarted/Stopped` - Monitor lifecycle events

### 2. YAML Configuration Updates
**Files modified**:
- `internal/task/manager.go` - Updated to parse new protocol fields
- `configs/tasks.yaml` - Can now include optional `protocol`, `asset_type`, and `metadata` fields

**Example**:
```yaml
tasks:
  - task_name: "NFT Mint"
    protocol: "nft"      # New field
    asset_type: "nft"    # New field
    metadata:            # New field
      collection: "..."
      rarity: "rare"
```

## Future Deprecation Plan

### Phase 2: Replace DEX-specific code with Protocol abstraction
**Files/functions to eventually deprecate**:
1. `internal/dex/factory.go:GetDEXByName()` 
   - Replace with: `protocol.Registry.Get(name)`
   
2. Direct DEX interface usage in `worker.go`
   - Replace with: Protocol interface operations

3. `internal/bot/worker.go:handleTask()` DEX-specific logic
   - Replace with: Protocol-agnostic operation execution

### Phase 3: Unified Operation Interface
**Current code to replace**:
1. `task.Operation` enum (OperationSnipe, OperationSwap, OperationSell)
   - Replace with: Dynamic operation types from Protocol.GetCapabilities()

2. Operation-specific handling in worker
   - Replace with: Generic Operation.Execute() pattern

3. `dex.DEX.Execute(task)` method
   - Replace with: `protocol.Operation.Execute(ctx)`

### Phase 4: Remove legacy structures
**Potential removals**:
1. `internal/dex/types.go` - DEX interface
   - Functionality absorbed by Protocol interface
   
2. Module-based routing (`task.Module` field)
   - Replace with: `task.Protocol` field entirely

3. DEX-specific calculators in `internal/monitor/`
   - Replace with: Protocol-specific price providers

## Migration Strategy

### Step 1: Parallel Systems (Current State)
- Both DEX and Protocol systems coexist
- DEXAdapter wraps existing DEX implementations
- No breaking changes to existing functionality

### Step 2: Gradual Migration
```go
// Current code
dexAdapter, err := dex.GetDEXByName(t.Module, ...)

// Future code
protocol, err := protocol.Registry.Get(t.GetProtocol())
operation, err := protocol.CreateOperation(ctx, params)
```

### Step 3: Feature Parity
Before removing old code, ensure:
- All DEX features work through Protocol interface
- Performance is equivalent or better
- All tests pass with new implementation

### Step 4: Cleanup
Remove deprecated code after successful migration and testing period.

## Benefits of Changes

1. **Extensibility**: Easy to add NFT, staking, lending protocols
2. **Decoupling**: Components communicate through events, not direct calls
3. **Testability**: Mock protocols and event handlers for testing
4. **Observability**: Event bus provides central point for monitoring
5. **Backward Compatibility**: Existing configurations continue to work

## Testing Recommendations

1. **Event Bus Testing**:
   - Verify events are published for all operations
   - Test event bus performance under load
   - Ensure no event loss during shutdown

2. **Protocol Adapter Testing**:
   - Verify DEXAdapter correctly wraps all DEX methods
   - Test error handling and edge cases
   - Benchmark performance overhead

3. **Integration Testing**:
   - Run existing trading scenarios
   - Verify monitoring still works correctly
   - Check that all events are properly logged

## Next Steps

1. Implement WebSocket support using event bus for real-time updates
2. Create NFT protocol implementation as proof of concept
3. Build API layer that subscribes to events
4. Develop metrics collector using event statistics
5. Create protocol-specific UI components

## Rollback Plan

If issues arise, the changes can be rolled back by:
1. Removing event publishing calls (they don't affect core logic)
2. Removing protocol fields from Task (backward compatible)
3. Deleting new packages (events/, protocol/)
4. No changes to core trading logic were made

The refactoring was designed to be additive, ensuring the existing system continues to function while new capabilities are built alongside.