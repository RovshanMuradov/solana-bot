// internal/eventlistener/listener.go
package eventlistener

import (
	"context"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
)

type EventListener struct {
	conn   net.Conn
	logger *zap.Logger
}

type Event struct {
	// Определите поля события
}

func NewEventListener(ctx context.Context, wsURL string, logger *zap.Logger) (*EventListener, error) {
	// Устанавливаем WebSocket-соединение с использованием контекста
	conn, _, _, err := ws.Dial(ctx, wsURL)
	if err != nil {
		return nil, err
	}

	return &EventListener{
		conn:   conn,
		logger: logger,
	}, nil
}

func (el *EventListener) Subscribe(handler func(event Event)) error {
	// Запускаем цикл чтения сообщений
	go func() {
		for {
			// Читаем сообщение из WebSocket
			_, _, err := wsutil.ReadServerData(el.conn)
			if err != nil {
				// Логируем ошибку и продолжаем или завершаем цикл
				el.logger.Error("Ошибка чтения из WebSocket", zap.Error(err))
				break
			}

			// Парсим сообщение в структуру Event
			var event Event
			// Преобразование msg в event

			// Вызываем обработчик
			handler(event)
		}
	}()

	return nil
}
