// internal/wallet/wallet.go
package wallet

import (
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"os"

	"github.com/gagliardetto/solana-go"
)

// Wallet представляет кошелек Solana
type Wallet struct {
	PrivateKey solana.PrivateKey
	PublicKey  solana.PublicKey
}

// NewWallet создает новый кошелек из Base64-строки приватного ключа
func NewWallet(privateKeyBase64 string) (*Wallet, error) {
	// Декодируем Base64 строку в байты
	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}

	// Создаем приватный ключ
	privateKey := solana.PrivateKey(privateKeyBytes)

	// Получаем публичный ключ
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

	// Пропускаем заголовок и проверяем наличие данных
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file is empty or contains only header")
	}

	wallets := make(map[string]*Wallet)

	// Обрабатываем каждую строку, начиная со второй (пропускаем заголовок)
	for _, record := range records[1:] {
		if len(record) != 2 {
			continue // Пропускаем некорректные строки
		}

		name := record[0]
		wallet, err := NewWallet(record[1])
		if err != nil {
			continue // Пропускаем некорректные кошельки
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
