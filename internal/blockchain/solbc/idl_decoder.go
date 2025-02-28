package solbc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// IDLDecoder helps decode and work with Anchor program IDLs
type IDLDecoder struct {
	logger *zap.Logger
	client *rpc.Client
}

// AnchorIDL represents the structure of an Anchor IDL
type AnchorIDL struct {
	Version   string   `json:"version"`
	Name      string   `json:"name"`
	Metadata  Metadata `json:"metadata"`
	Functions []struct {
		Name    string `json:"name"`
		Args    []Arg  `json:"args"`
		Returns any    `json:"returns,omitempty"`
	} `json:"instructions"`
	Accounts []any  `json:"accounts"`
	Types    []Type `json:"types"`
}

// Metadata is part of the IDL
type Metadata struct {
	Address string `json:"address"`
}

// Arg represents a function argument
type Arg struct {
	Name string `json:"name"`
	Type any    `json:"type"`
}

// Type represents a custom type definition
type Type struct {
	Name     string `json:"name"`
	Type     any    `json:"type"`
	Fields   []any  `json:"fields,omitempty"`
	Variants []any  `json:"variants,omitempty"`
}

// NewIDLDecoder creates a new IDL decoder
func NewIDLDecoder(client *rpc.Client, logger *zap.Logger) *IDLDecoder {
	return &IDLDecoder{
		client: client,
		logger: logger.Named("idl-decoder"),
	}
}

// FetchIDL tries to fetch an IDL for a program from Anchor's IDL repository
func (d *IDLDecoder) FetchIDL(ctx context.Context, programID string) (*AnchorIDL, error) {
	// Try local file first if available
	idl, err := d.loadLocalIDL(programID)
	if err == nil {
		d.logger.Debug("Loaded IDL from local file",
			zap.String("program", programID))
		return idl, nil
	}

	// Try to fetch from Solana (on-chain IDL account)
	idl, err = d.fetchOnChainIDL(ctx, programID)
	if err == nil {
		d.logger.Debug("Loaded IDL from on-chain account",
			zap.String("program", programID))
		return idl, nil
	}

	// Try to fetch from known Anchor IDL repositories
	idl, err = d.fetchFromRepository(programID)
	if err == nil {
		// Save for future use
		d.saveLocalIDL(programID, idl)
		d.logger.Debug("Loaded IDL from repository",
			zap.String("program", programID))
		return idl, nil
	}

	return nil, fmt.Errorf("failed to fetch IDL for program %s: %w", programID, err)
}

// GetDiscriminators extracts method discriminators from an IDL
func (d *IDLDecoder) GetDiscriminators(idl *AnchorIDL) map[string][]byte {
	discriminators := make(map[string][]byte)

	if idl == nil || len(idl.Functions) == 0 {
		return discriminators
	}

	// Extract method names and calculate discriminators
	for _, fn := range idl.Functions {
		discriminator := d.CalculateDiscriminator(fn.Name)
		discriminators[fn.Name] = discriminator

		// Also try with prefixes which are common in some Anchor programs
		discriminators["global:"+fn.Name] = d.CalculateDiscriminator("global:" + fn.Name)
	}

	return discriminators
}

// CalculateDiscriminator calculates an Anchor method discriminator
func (d *IDLDecoder) CalculateDiscriminator(methodName string) []byte {
	hash := sha256.Sum256([]byte(methodName))
	return hash[:8]
}

// loadLocalIDL attempts to load an IDL from a local file
func (d *IDLDecoder) loadLocalIDL(programID string) (*AnchorIDL, error) {
	filename := fmt.Sprintf("configs/idl/%s.json", programID)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var idl AnchorIDL
	if err := json.Unmarshal(data, &idl); err != nil {
		return nil, err
	}

	return &idl, nil
}

// saveLocalIDL saves an IDL to a local file
func (d *IDLDecoder) saveLocalIDL(programID string, idl *AnchorIDL) error {
	// Ensure directory exists
	if err := os.MkdirAll("configs/idl", 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("configs/idl/%s.json", programID)
	data, err := json.MarshalIndent(idl, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0644)
}

// fetchOnChainIDL attempts to fetch an IDL from an on-chain account
func (d *IDLDecoder) fetchOnChainIDL(ctx context.Context, programID string) (*AnchorIDL, error) {
	// On-chain IDL accounts follow a specific pattern
	// This is a simplified implementation - real one would need to derive the correct PDA

	programKey, err := solana.PublicKeyFromBase58(programID)
	if err != nil {
		return nil, err
	}

	// Calculate IDL account address (Anchor uses a specific PDA derivation)
	idlAddress, _, _ := solana.FindProgramAddress(
		[][]byte{[]byte("anchor:idl"), programKey.Bytes()},
		solana.MustPublicKeyFromBase58("IdLend19W6QEUKudyU73w5fQrUEpZ5ghSPi7YjPBrfqKP"),
	)

	// Fetch the account data
	account, err := d.client.GetAccountInfo(ctx, idlAddress)
	if err != nil {
		return nil, err
	}

	if account == nil || account.Value == nil {
		return nil, fmt.Errorf("IDL account not found")
	}

	// IDL data is stored as JSON
	data := account.Value.Data.GetBinary()

	// First 8 bytes are a header, actual JSON starts after
	if len(data) <= 8 {
		return nil, fmt.Errorf("invalid IDL data length")
	}

	var idl AnchorIDL
	if err := json.Unmarshal(data[8:], &idl); err != nil {
		return nil, err
	}

	return &idl, nil
}

// fetchFromRepository attempts to fetch an IDL from known repositories
func (d *IDLDecoder) fetchFromRepository(programID string) (*AnchorIDL, error) {
	// List of repositories to try
	repos := []string{
		fmt.Sprintf("https://api.apr.dev/api/idl/%s", programID),
		fmt.Sprintf("https://raw.githubusercontent.com/coral-xyz/anchor/master/ts/packages/anchor/src/idl/%s.json", programID),
	}

	var lastErr error
	for _, url := range repos {
		idl, err := d.fetchIDLFromURL(url)
		if err == nil {
			return idl, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

// fetchIDLFromURL fetches an IDL from a URL
func (d *IDLDecoder) fetchIDLFromURL(url string) (*AnchorIDL, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var idl AnchorIDL
	if err := json.Unmarshal(data, &idl); err != nil {
		return nil, err
	}

	return &idl, nil
}

// ScanForDiscriminators performs a brute-force scan to discover method discriminators
// by trying common Anchor naming patterns
func (d *IDLDecoder) ScanForDiscriminators(methodPatterns []string) map[string][]byte {
	results := make(map[string][]byte)

	d.logger.Info("Scanning for discriminators with common patterns",
		zap.Int("pattern_count", len(methodPatterns)))

	// Generate discriminators for each pattern
	for _, pattern := range methodPatterns {
		discriminator := d.CalculateDiscriminator(pattern)
		results[pattern] = discriminator
		d.logger.Debug("Generated discriminator",
			zap.String("method", pattern),
			zap.String("discriminator", hex.EncodeToString(discriminator)))
	}

	return results
}

// GenerateMethodPatterns generates common method naming patterns to try
func (d *IDLDecoder) GenerateMethodPatterns(baseNames []string) []string {
	// Common method name variations to try
	prefixes := []string{"", "global:", "app:", "token:", "market:"}
	casings := []func(string) string{
		// camelCase
		func(s string) string { return s },
		// snake_case
		func(s string) string { return strings.ReplaceAll(strings.ToLower(s), " ", "_") },
		// PascalCase
		func(s string) string {
			parts := strings.Split(s, " ")
			for i, part := range parts {
				if len(part) > 0 {
					parts[i] = strings.ToUpper(part[:1]) + part[1:]
				}
			}
			return strings.Join(parts, "")
		},
	}

	var patterns []string
	for _, base := range baseNames {
		for _, prefix := range prefixes {
			for _, casing := range casings {
				pattern := prefix + casing(base)
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns
}
