package wallet

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

// Wallet представляет кошелек Solana
type Wallet struct {
	PrivateKey solana.PrivateKey
	PublicKey  solana.PublicKey
}

// NewWallet создает новый кошелек из Base58-строки приватного ключа
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

// LoadWallets загружает кошельки из CSV файла
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
		return nil, fmt.Errorf("CSV file is empty or contains only header")
	}

	wallets := make(map[string]*Wallet)
	for _, record := range records[1:] {
		if len(record) != 2 {
			continue
		}

		name := record[0]
		wallet, err := NewWallet(record[1])
		if err != nil {
			continue
		}

		wallets[name] = wallet
	}

	return wallets, nil
}

// SignTransaction подписывает транзакцию
func (w *Wallet) SignTransaction(tx *solana.Transaction) error {
	_, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(w.PublicKey) {
			return &w.PrivateKey
		}
		return nil
	})
	return err
}

// GetATA возвращает адрес ассоциированного токен аккаунта
func (w *Wallet) GetATA(mint solana.PublicKey) (solana.PublicKey, error) {
	ata, _, err := solana.FindAssociatedTokenAddress(w.PublicKey, mint)
	return ata, err
}

// String возвращает строковое представление адреса кошелька
func (w *Wallet) String() string {
	return w.PublicKey.String()
}
