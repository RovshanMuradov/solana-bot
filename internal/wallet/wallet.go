package wallet

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"

	"github.com/gagliardetto/solana-go"
)

type Wallet struct {
	Name       string
	PrivateKey solana.PrivateKey
	PublicKey  solana.PublicKey
	Address    string
}

func LoadWallets(path string) (map[string]*Wallet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, errors.New("CSV file is empty or contains only header")
	}

	wallets := make(map[string]*Wallet)
	for _, record := range records[1:] { // Пропускаем заголовок
		if len(record) != 2 {
			return nil, errors.New("invalid CSV format")
		}

		name := record[0]
		privateKeyStr := record[1]

		privateKey, err := solana.PrivateKeyFromBase58(privateKeyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid private key for wallet %s: %v", name, err)
		}

		publicKey := privateKey.PublicKey()
		address := publicKey.String()

		wallets[name] = &Wallet{
			Name:       name,
			PrivateKey: privateKey,
			PublicKey:  publicKey,
			Address:    address,
		}
	}

	if len(wallets) == 0 {
		return nil, errors.New("no valid wallets found in CSV file")
	}

	return wallets, nil
}

func (w *Wallet) SignTransaction(tx *solana.Transaction) error {
	_, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(w.PublicKey) {
			return &w.PrivateKey
		}
		return nil
	})
	return err
}
