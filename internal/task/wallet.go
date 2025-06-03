// ==================================
// File: internal/task/wallet.go
// ==================================
package task

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gagliardetto/solana-go"
	"gopkg.in/yaml.v3"
)

// Wallet представляет кошелёк Solana.
type Wallet struct {
	PrivateKey solana.PrivateKey
	PublicKey  solana.PublicKey
	ATACache   map[string]solana.PublicKey // Кеш для ассоциированных адресов токен-аккаунтов (ATA)
}

// NewWallet создаёт новый кошелёк из base58-encoded приватного ключа.
func NewWallet(privateKeyBase58 string) (*Wallet, error) {
	privateKey, err := solana.PrivateKeyFromBase58(privateKeyBase58)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	publicKey := privateKey.PublicKey()

	return &Wallet{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		ATACache:   make(map[string]solana.PublicKey),
	}, nil
}

// WalletConfig represents the structure of wallets YAML file
type WalletConfig struct {
	Wallets []struct {
		Name       string `yaml:"name"`
		PrivateKey string `yaml:"private_key"`
	} `yaml:"wallets"`
}

// LoadWallets загружает кошельки из YAML-файла.
func LoadWallets(path string) (map[string]*Wallet, error) {
	// Clean the path to prevent path traversal issues
	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config WalletConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if len(config.Wallets) == 0 {
		return nil, fmt.Errorf("no wallets found in configuration")
	}

	wallets := make(map[string]*Wallet)
	for _, walletData := range config.Wallets {
		if walletData.Name == "" || walletData.PrivateKey == "" {
			continue
		}
		w, err := NewWallet(walletData.PrivateKey)
		if err != nil {
			continue
		}
		wallets[walletData.Name] = w
	}

	if len(wallets) == 0 {
		return nil, fmt.Errorf("no valid wallets loaded")
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

// CreateAssociatedTokenAccountIdempotentInstruction создает инструкцию для создания ассоциированного токен-аккаунта
func (w *Wallet) CreateAssociatedTokenAccountIdempotentInstruction(payer, wallet, mint solana.PublicKey) solana.Instruction {
	ata, _, err := solana.FindAssociatedTokenAddress(wallet, mint)
	if err != nil {
		panic(fmt.Sprintf("failed to find associated token address: %v", err))
	}

	return solana.NewInstruction(
		solana.SPLAssociatedTokenAccountProgramID,
		[]*solana.AccountMeta{
			solana.Meta(payer).WRITE().SIGNER(),
			solana.Meta(ata).WRITE(),
			solana.Meta(wallet),
			solana.Meta(mint),
			solana.Meta(solana.SystemProgramID),
			solana.Meta(solana.TokenProgramID),
			solana.Meta(solana.SysVarRentPubkey),
		},
		[]byte{1}, // 1 = create_idempotent
	)
}

// String возвращает строковое представление кошелька (его публичный ключ).
func (w *Wallet) String() string {
	return w.PublicKey.String()
}
