// internal/eventlistener/types.go

// Этот файл содержит определение типов для пакета eventlistener
package eventlistener

import (
	"net"

	"go.uber.org/zap"
)

// EventListener представляет слушатель событий WebSocket
type EventListener struct {
	conn   net.Conn    // WebSocket соединение
	logger *zap.Logger // Логгер для записи событий и ошибок
	wsURL  string      // URL WebSocket сервера
}

// Event представляет собой структуру события, полученного от WebSocket
type Event struct {
	Type   string `json:"type"`              // Тип события
	PoolID string `json:"pool_id,omitempty"` // ID пула (если применимо)
	TokenA string `json:"token_a,omitempty"` // Первый токен в паре (если применимо)
	TokenB string `json:"token_b,omitempty"` // Второй токен в паре (если применимо)
	// Добавьте другие необходимые поля здесь
}
