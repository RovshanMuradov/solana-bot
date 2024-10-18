package eventlistener

import (
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

func NewEventListener(wsURL string, logger *zap.Logger) (*EventListener, error) {
	// Устанавливаем WebSocket-соединение
	conn, _, _, err := ws.Dial(nil, wsURL)
	if err != nil {
		// Возвращаем ошибку подключения
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
			msg, op, err := wsutil.ReadServerData(el.conn)
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
