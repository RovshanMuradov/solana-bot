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

// Wallet представляет кошелёк Solana.
type Wallet struct {
	PrivateKey solana.PrivateKey
	PublicKey  solana.PublicKey
	ATACache   map[string]solana.PublicKey // Кеш для ассоциированных адресов токен-аккаунтов (ATA)
}

// NewWallet создаёт новый кошелёк из base58-encoded приватного ключа.
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
		ATACache:   make(map[string]solana.PublicKey),
	}, nil
}

// LoadWallets загружает кошельки из CSV-файла с колонками: [Name, PrivateKeyBase58].
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

// SignTransaction подписывает транзакцию с помощью приватного ключа кошелька.
func (w *Wallet) SignTransaction(tx *solana.Transaction) error {
	_, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(w.PublicKey) {
			return &w.PrivateKey
		}
		return nil
	})
	return err
}

// GetATA возвращает адрес ассоциированного токен-аккаунта (ATA) для заданного токена (mint).
// Если адрес уже был вычислен ранее, возвращается значение из кеша.
func (w *Wallet) GetATA(mint solana.PublicKey) (solana.PublicKey, error) {
	mintStr := mint.String()
	if ata, ok := w.ATACache[mintStr]; ok {
		return ata, nil
	}
	ata, _, err := solana.FindAssociatedTokenAddress(w.PublicKey, mint)
	if err != nil {
		return solana.PublicKey{}, err
	}
	// Сохраняем вычисленный ATA в кеш
	w.ATACache[mintStr] = ata
	return ata, nil
}

// PrecomputeATAs позволяет заранее рассчитать ATA для списка токенов.
// Эту функцию можно вызвать при запуске бота, чтобы все ATA были рассчитаны сразу.
func (w *Wallet) PrecomputeATAs(mints []solana.PublicKey) error {
	for _, mint := range mints {
		if _, err := w.GetATA(mint); err != nil {
			return fmt.Errorf("failed to precompute ATA for mint %s: %w", mint.String(), err)
		}
	}
	return nil
}

// createAssociatedTokenAccountIdempotentInstruction creates an instruction to create an associated token account
func (w *Wallet) CreateAssociatedTokenAccountIdempotentInstruction(payer, wallet, mint solana.PublicKey) solana.Instruction {
	associatedTokenProgramID := solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

	// Calculate the associated token account address
	ata, _, _ := solana.FindAssociatedTokenAddress(wallet, mint)

	return solana.NewInstruction(
		associatedTokenProgramID,
		[]*solana.AccountMeta{
			{PublicKey: payer, IsWritable: true, IsSigner: true},
			{PublicKey: ata, IsWritable: true, IsSigner: false},
			{PublicKey: wallet, IsWritable: false, IsSigner: false},
			{PublicKey: mint, IsWritable: false, IsSigner: false},
			{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false},
			{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},
			{PublicKey: solana.SysVarRentPubkey, IsWritable: false, IsSigner: false},
		},
		[]byte{1}, // Instruction code 1 for create idempotent
	)
}

// String возвращает строковое представление кошелька (его публичный ключ).
func (w *Wallet) String() string {
	return w.PublicKey.String()
}
