// inernal/dex/raydium/types.go - это пакет, который содержит в себе реализацию работы с декстерами Raydium
package raydium

import "github.com/gagliardetto/solana-go"

type RaydiumPool struct {
	ID            solana.PublicKey // Идентификатор пула
	Authority     solana.PublicKey // Публичный ключ, который имеет полномочия управлять пулом
	BaseMint      solana.PublicKey // Публичный ключ базового токена
	QuoteMint     solana.PublicKey // Публичный ключ котируемого токена
	BaseVault     solana.PublicKey // Публичный ключ хранилища базового токена
	QuoteVault    solana.PublicKey // Публичный ключ хранилища котируемого токена
	BaseDecimals  uint8            // Количество десятичных знаков базового токена
	QuoteDecimals uint8            // Количество десятичных знаков котируемого токена
	DefaultFeeBps uint16           // Комиссия по умолчанию в базисных пунктах (bps)
	// Только необходимые поля для V4
}

type PoolState struct {
	BaseReserve  uint64 // Резерв базового токена в пуле
	QuoteReserve uint64 // Резерв котируемого токена в пуле
	Status       uint8  // Статус пула (например, активен или неактивен)
}

type SwapParams struct {
	UserWallet              solana.PublicKey   // Публичный ключ кошелька пользователя
	PrivateKey              *solana.PrivateKey // Приватный ключ для подписания транзакции
	AmountIn                uint64             // Количество входного токена для обмена
	MinAmountOut            uint64             // Минимальное количество выходного токена
	Pool                    *RaydiumPool       // Указатель на пул для обмена
	SourceTokenAccount      solana.PublicKey   // Аккаунт исходного токена
	DestinationTokenAccount solana.PublicKey   // Аккаунт целевого токена
	PriorityFeeLamports     uint64             // Приоритетная комиссия в лампортах
}

// Основные ошибки
type SwapError struct {
	Stage   string // Этап, на котором произошла ошибка
	Message string // Сообщение об ошибке
	Err     error  // Вложенная ошибка
}
