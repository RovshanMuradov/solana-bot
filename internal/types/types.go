// internal/types/types.go
package types

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

type Task struct {
	TaskName                    string
	Module                      string
	Workers                     int
	WalletName                  string
	Delta                       int
	PriorityFee                 float64
	SourceToken                 string
	TargetToken                 string
	AmountIn                    float64
	AutosellPercent             float64
	AutosellDelay               int
	AutosellAmount              float64
	TransactionDelay            int
	AutosellPriorityFee         float64
	UserSourceTokenAccount      solana.PublicKey
	UserDestinationTokenAccount solana.PublicKey
	SourceTokenDecimals         int
	TargetTokenDecimals         int
	DEXName                     string `default:"Raydium"` // Добавляем значение по умолчанию
	SlippageConfig              SlippageConfig
}

type DEX interface {
	// Возвращает имя DEX
	GetName() string
	// Возвращает базовый клиент для работы с блокчейном
	GetClient() blockchain.Client
	// Возвращает конфигурацию DEX
	GetConfig() interface{}
	// Выполняет свап (новый метод)
	ExecuteSwap(ctx context.Context, task *Task, wallet *wallet.Wallet) error
}

type Blockchain interface {
	Name() string
	SendTransaction(ctx context.Context, tx interface{}) (string, error)
}
