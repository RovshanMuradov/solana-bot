package wallet

import (
	"io/ioutil"

	"github.com/gagliardetto/solana-go"
)

type Wallet struct {
	Name       string
	PrivateKey solana.PrivateKey
	PublicKey  solana.PublicKey
	Address    string
}

func LoadWallets(path string) (map[string]*Wallet, error) {
	// Открытие файла wallets.csv
	data, err := ioutil.ReadFile(path)
	if err != nil {
		// Возвращаем ошибку чтения файла
		return nil, err
	}

	// Парсинг CSV данных
	// Создание map[string]*Wallet
	wallets := make(map[string]*Wallet)

	// Для каждой строки в CSV:
	// - Извлекаем имя кошелька и приватный ключ
	// - Декодируем приватный ключ из строки
	// - Создаем объект Wallet и добавляем в map

	return wallets, nil
}

func (w *Wallet) SignTransaction(tx *solana.Transaction) error {
	// Используем приватный ключ для подписания транзакции
	// Добавляем подпись в транзакцию
	return nil
}
