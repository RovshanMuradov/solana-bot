// ==================================
// File: internal/wallet/wallet.go
// ==================================
package wallet

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

// Wallet represents a Solana wallet.
type Wallet struct {
	PrivateKey solana.PrivateKey
	PublicKey  solana.PublicKey
}

// NewWallet creates a new wallet from a base58-encoded private key.
func NewWallet(privateKeyBase58 string) (*Wallet, error) {
	privateKeyBytes, err := base58.Decode(privateKeyBase58)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}
	if len(privateKeyBytes) != 64 {
		return nil, fmt.Errorf("invalid private key length: expected 64 bytes, got %d", len(privateKeyBytes))
	}
	privateKey := solana.PrivateKey(privateKeyBytes)
	publicKey := privateKey.PublicKey()
	return &Wallet{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}

// LoadWallets loads wallets from a CSV file with columns: [Name, PrivateKeyBase58].
func LoadWallets(path string) (map[string]*Wallet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file is empty or missing data")
	}

	wallets := make(map[string]*Wallet)
	for _, record := range records[1:] {
		if len(record) != 2 {
			continue
		}
		name := record[0]
		w, err := NewWallet(record[1])
		if err != nil {
			continue
		}
		wallets[name] = w
	}
	return wallets, nil
}

// SignTransaction signs a transaction with the wallet's private key.
func (w *Wallet) SignTransaction(tx *solana.Transaction) error {
	_, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(w.PublicKey) {
			return &w.PrivateKey
		}
		return nil
	})
	return err
}

// GetATA returns the associated token account address for a given mint.
func (w *Wallet) GetATA(mint solana.PublicKey) (solana.PublicKey, error) {
	ata, _, err := solana.FindAssociatedTokenAddress(w.PublicKey, mint)
	return ata, err
}

// String returns a string representation of the wallet (public key).
func (w *Wallet) String() string {
	return w.PublicKey.String()
}
