// ==============================================
// File: internal/dex/pumpfun/checker.go
// ==============================================
package pumpfun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// Constants for account checking operations
const (
	// Maximum number of retries for account validation
	MaxRetries = 3

	// Timeout values
	AccountCheckTimeout   = 5 * time.Second
	DefaultCheckerTimeout = 30 * time.Second
)

// ProgramStateChecker provides utilities for checking and monitoring PumpFun program state
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

// WithTimeout sets a custom timeout for operations
func (p *ProgramStateChecker) WithTimeout(timeout time.Duration) *ProgramStateChecker {
	p.timeout = timeout
	return p
}

// CheckProgramState verifies the PumpFun program state and its key accounts
func (p *ProgramStateChecker) CheckProgramState(ctx context.Context) (*ProgramState, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	p.logger.Info("Checking PumpFun program state",
		zap.String("program_id", p.config.ContractAddress.String()),
		zap.String("token_mint", p.config.Mint.String()),
		zap.Duration("timeout", p.timeout))

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
		// Check if account not found - look for strings in the error message
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "Not Found") {
			p.logger.Warn("Global account not found",
				zap.String("account", p.config.Global.String()))
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

	// Additional checks for other accounts
	p.checkAdditionalAccounts(ctxWithTimeout, state)

	return state, nil
}

// checkAdditionalAccounts performs additional validations on related accounts
func (p *ProgramStateChecker) checkAdditionalAccounts(ctx context.Context, state *ProgramState) {
	// Check bonding curve
	bondingInfo, err := p.client.GetAccountInfo(ctx, p.config.BondingCurve)
	if err == nil && bondingInfo.Value != nil {
		state.BondingCurveInitialized = true
		state.BondingCurveOwner = bondingInfo.Value.Owner.String()
	}

	// Check associated bonding curve
	assocInfo, err := p.client.GetAccountInfo(ctx, p.config.AssociatedBondingCurve)
	if err == nil && assocInfo.Value != nil {
		state.AssociatedCurveInitialized = true
		state.AssociatedCurveOwner = assocInfo.Value.Owner.String()
	}
}

// FindAlternativeGlobalAccount attempts to find the correct global account if the configured one is invalid
func (p *ProgramStateChecker) FindAlternativeGlobalAccount(ctx context.Context) (string, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	p.logger.Info("Attempting to find alternative global account",
		zap.String("program_id", p.config.ContractAddress.String()))

	// We don't have GetProgramAccountsWithOpts, so we need to implement our own logic
	// For now, let's try a simple heuristic approach to find potential global accounts

	// First, check if we can use known patterns to derive the global account
	candidates := p.generateGlobalAccountCandidates()

	p.logger.Info("Checking candidate global accounts",
		zap.Int("count", len(candidates)))

	// Check each candidate
	for _, candidate := range candidates {
		candidatePK, err := solana.PublicKeyFromBase58(candidate)
		if err != nil {
			continue
		}

		accountInfo, err := p.client.GetAccountInfo(ctxWithTimeout, candidatePK)
		if err != nil {
			continue
		}

		if accountInfo.Value != nil && accountInfo.Value.Owner.Equals(p.config.ContractAddress) {
			p.logger.Info("Found potential alternative global account",
				zap.String("account", candidate))
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no suitable global account candidate found")
}

// generateGlobalAccountCandidates generates potential global account candidates
// based on common patterns and known derivation methods
func (p *ProgramStateChecker) generateGlobalAccountCandidates() []string {
	// Common naming patterns and derivation methods
	// This is a simplified implementation
	candidates := []string{
		// Try close variants of the current global account (maybe off-by-one character)
		p.config.Global.String(),

		// Try PDA derivation with common seeds
		// This would be a list of PDA derivations if we knew the seed pattern
	}

	// Add more candidates based on your knowledge of the protocol

	return candidates
}

// GetAccountBalance gets the SOL balance of an account
func (p *ProgramStateChecker) GetAccountBalance(ctx context.Context, account solana.PublicKey) (uint64, error) {
	// Add required commitment parameter
	balance, err := p.client.GetBalance(ctx, account, rpc.CommitmentConfirmed)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

// ProgramState represents the state of the PumpFun program and its key accounts
type ProgramState struct {
	ProgramID                  string `json:"program_id"`
	IsExecutable               bool   `json:"is_executable"`
	GlobalAccount              string `json:"global_account"`
	GlobalInitialized          bool   `json:"global_initialized"`
	GlobalOwner                string `json:"global_owner,omitempty"`
	BondingCurve               string `json:"bonding_curve"`
	BondingCurveInitialized    bool   `json:"bonding_curve_initialized"`
	BondingCurveOwner          string `json:"bonding_curve_owner,omitempty"`
	AssociatedCurve            string `json:"associated_curve,omitempty"`
	AssociatedCurveInitialized bool   `json:"associated_curve_initialized"`
	AssociatedCurveOwner       string `json:"associated_curve_owner,omitempty"`
	TokenMint                  string `json:"token_mint"`
	Error                      string `json:"error,omitempty"`
}

// String returns a JSON representation of the program state
func (s *ProgramState) String() string {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling program state: %v", err)
	}
	return string(data)
}

// IsReady returns true if the program is ready for operations
func (s *ProgramState) IsReady() bool {
	return s.IsExecutable && s.GlobalInitialized && s.BondingCurveInitialized
}
