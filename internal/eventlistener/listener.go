// internal/eventlistener/listener.go

// Этот файл содержит логику для установки соединения, подписки на события и переподключения.
package eventlistener

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
)

func NewEventListener(ctx context.Context, wsURL string, logger *zap.Logger) (*EventListener, error) {
	// Устанавливаем WebSocket-соединение с использованием контекста
	conn, _, _, err := ws.Dial(ctx, wsURL)
	if err != nil {
		return nil, err
	}

	return &EventListener{
		conn:   conn,
		logger: logger,
		wsURL:  wsURL,
	}, nil
}

func (el *EventListener) Subscribe(ctx context.Context, handler func(event Event)) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				el.logger.Info("Подписка отменена")
				return
			default:
				msg, _, err := wsutil.ReadServerData(el.conn)
				if err != nil {
					el.logger.Error("Ошибка чтения из WebSocket", zap.Error(err))
					if err = el.reconnect(); err != nil {
						el.logger.Error("Не удалось переподключиться", zap.Error(err))
						return
					}
					continue
				}

				var event Event
				if err := json.Unmarshal(msg, &event); err != nil {
					el.logger.Error("Ошибка парсинга сообщения", zap.Error(err))
					continue
				}

				handler(event)
			}
		}
	}()

	return nil
}

func (el *EventListener) reconnect() error {
	// Закрываем текущее соединение
	el.conn.Close()

	// Пытаемся установить новое соединение
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newConn, _, _, err := ws.Dial(ctx, el.wsURL)
	if err != nil {
		return err
	}

	el.conn = newConn
	return nil
}
