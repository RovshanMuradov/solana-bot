// =============================
// File: internal/dex/factory.go
// =============================
package dex

import (
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
	"strings"
)

// GetDEXByName создаёт адаптер для DEX по имени биржи.
//
// Метод возвращает соответствующий адаптер DEX на основе предоставленного имени биржи.
// Поддерживаемые биржи: "pump.fun" и "pump.swap". Имя биржи обрабатывается без учёта
// регистра и с удалением лишних пробелов.
//
// Параметры:
//   - name: строковое имя биржи (например, "pump.fun" или "pump.swap")
//   - client: клиент Solana блокчейна для взаимодействия с сетью
//   - w: кошелёк пользователя для подписи транзакций
//   - logger: логгер для записи информационных и отладочных сообщений
//
// Возвращает:
//   - DEX: интерфейс для взаимодействия с выбранной биржей
//   - error: ошибку, если биржа не поддерживается или если один из обязательных параметров равен nil
func GetDEXByName(name string, client *solbc.Client, w *wallet.Wallet, logger *zap.Logger) (DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet cannot be nil")
	}

	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "pump.fun":
		return &pumpfunDEXAdapter{
			baseDEXAdapter: baseDEXAdapter{
				client: client,
				wallet: w,
				logger: logger,
				name:   "Pump.fun",
			},
		}, nil

	case "pump.swap":
		return &pumpswapDEXAdapter{
			baseDEXAdapter: baseDEXAdapter{
				client: client,
				wallet: w,
				logger: logger,
				name:   "Pump.Swap",
			},
		}, nil

	default:
		return nil, fmt.Errorf("exchange %s is not supported", name)
	}
}
