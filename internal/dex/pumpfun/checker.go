// ==============================================
// File: internal/dex/pumpfun/checker.go
// ==============================================
package pumpfun

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// Constants for account checking operations
const (
	DefaultCheckerTimeout = 30 * time.Second
)

// ProgramStateChecker provides utilities for checking Pump.fun program state
type ProgramStateChecker struct {
	client  *solbc.Client
	config  *Config
	logger  *zap.Logger
	timeout time.Duration
}

// NewProgramStateChecker creates a new PumpFun program state checker
func NewProgramStateChecker(client *solbc.Client, config *Config, logger *zap.Logger) *ProgramStateChecker {
	return &ProgramStateChecker{
		client:  client,
		config:  config,
		logger:  logger,
		timeout: DefaultCheckerTimeout,
	}
}

// CheckProgramState verifies the PumpFun program state and its key accounts
func (p *ProgramStateChecker) CheckProgramState(ctx context.Context) (*ProgramState, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Get program info
	programInfo, err := p.client.GetAccountInfo(ctxWithTimeout, p.config.ContractAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get program info: %w", err)
	}

	if programInfo.Value == nil || !programInfo.Value.Executable {
		return nil, fmt.Errorf("program account %s is not executable", p.config.ContractAddress.String())
	}

	// Check global account
	globalInfo, err := p.client.GetAccountInfo(ctxWithTimeout, p.config.Global)
	if err != nil {
		// Check if account not found
		if strings.Contains(err.Error(), "not found") {
			return &ProgramState{
				ProgramID:         p.config.ContractAddress.String(),
				IsExecutable:      true,
				GlobalAccount:     p.config.Global.String(),
				GlobalInitialized: false,
				Error:             "Global account not initialized",
			}, nil
		}
		return nil, fmt.Errorf("failed to get global account info: %w", err)
	}

	// Check if global account is owned by the program
	globalOwner := globalInfo.Value.Owner.String()
	globalInitialized := globalOwner == p.config.ContractAddress.String()

	state := &ProgramState{
		ProgramID:         p.config.ContractAddress.String(),
		IsExecutable:      true,
		GlobalAccount:     p.config.Global.String(),
		GlobalInitialized: globalInitialized,
		GlobalOwner:       globalOwner,
		BondingCurve:      p.config.BondingCurve.String(),
		TokenMint:         p.config.Mint.String(),
	}

	// If global account isn't initialized by the program, set error
	if !globalInitialized {
		state.Error = fmt.Sprintf("Global account not initialized by program (owner: %s)", globalOwner)
	}

	// Check bonding curve
	bondingInfo, err := p.client.GetAccountInfo(ctxWithTimeout, p.config.BondingCurve)
	if err == nil && bondingInfo.Value != nil {
		state.BondingCurveInitialized = true
		state.BondingCurveOwner = bondingInfo.Value.Owner.String()
	}

	// Check associated bonding curve
	assocInfo, err := p.client.GetAccountInfo(ctxWithTimeout, p.config.AssociatedBondingCurve)
	if err == nil && assocInfo.Value != nil {
		state.AssociatedCurveInitialized = true
		state.AssociatedCurveOwner = assocInfo.Value.Owner.String()
	}

	return state, nil
}

// IsReady returns true if the program is ready for operations
func (s *ProgramState) IsReady() bool {
	return s.IsExecutable && s.GlobalInitialized && s.BondingCurveInitialized
}
