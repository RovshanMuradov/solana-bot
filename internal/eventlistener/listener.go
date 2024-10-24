package eventlistener

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
)

func NewEventListener(ctx context.Context, wsURL string, logger *zap.Logger) (*EventListener, error) {
	conn, _, _, err := ws.Dial(ctx, wsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to dial websocket: %w", err)
	}

	return &EventListener{
		conn:   conn,
		logger: logger,
		wsURL:  wsURL,
		done:   make(chan struct{}),
		validator: &eventValidator{
			validTypes: map[string]bool{
				"NewPool":     true,
				"PriceChange": true,
			},
		},
	}, nil
}

func (el *EventListener) Subscribe(ctx context.Context, handler func(event Event)) error {
	go func() {
		defer el.Close()
		for {
			select {
			case <-ctx.Done():
				el.logger.Info("Context cancelled")
				return
			case <-el.done:
				el.logger.Info("Listener closed")
				return
			default:
				err := el.readAndHandleMessage(handler)
				if err != nil {
					el.logger.Info("Connection error, attempting reconnect", zap.Error(err))
					if err := el.reconnect(); err != nil {
						el.logger.Error("Failed to reconnect", zap.Error(err))
						return
					}
					el.logger.Info("Successfully reconnected and continuing message loop")
				}
			}
		}
	}()

	return nil
}

func (el *EventListener) readAndHandleMessage(handler func(event Event)) error {
	for {
		el.mu.Lock()
		conn := el.conn
		el.mu.Unlock()

		if conn == nil {
			return fmt.Errorf("connection is nil")
		}

		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			return fmt.Errorf("set read deadline: %w", err)
		}

		msg, op, err := wsutil.ReadServerData(conn)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				// Таймаут чтения, продолжаем
				continue
			}
			// Возвращаем ошибку для обработки в Subscribe
			return err
		}

		if op == ws.OpPing {
			el.logger.Debug("Received ping, sending pong")
			if err := wsutil.WriteClientMessage(conn, ws.OpPong, nil); err != nil {
				return fmt.Errorf("write pong: %w", err)
			}
			continue
		}

		if op != ws.OpText {
			el.logger.Debug("Skipping non-text message", zap.Int("opcode", int(op)))
			continue
		}

		var event Event
		if err := json.Unmarshal(msg, &event); err != nil {
			el.logger.Debug("Failed to parse message",
				zap.Error(err),
				zap.String("message", string(msg)))
			continue
		}

		if !el.validator.isValidEvent(event) {
			el.logger.Debug("Invalid event received",
				zap.String("type", event.Type))
			continue
		}

		el.logger.Debug("Valid event received, calling handler",
			zap.String("type", event.Type),
			zap.String("poolID", event.PoolID))

		handler(event)
	}
}

func (el *EventListener) reconnect() error {
	el.mu.Lock()
	if el.conn != nil {
		el.logger.Debug("Closing existing connection")
		el.conn.Close()
		el.conn = nil
	}
	el.mu.Unlock()

	backoff := initialBackoff

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-el.done:
			return errors.New("listener is closed")
		default:
			el.logger.Debug("Attempting to reconnect",
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff))

			ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
			conn, _, _, err := ws.Dial(ctx, el.wsURL)
			cancel()

			if err == nil {
				el.logger.Info("Successfully reconnected",
					zap.Int("attempt", attempt+1))
				el.mu.Lock()
				el.conn = conn
				el.mu.Unlock()
				time.Sleep(200 * time.Millisecond)
				return nil
			}

			el.logger.Debug("Reconnection attempt failed",
				zap.Int("attempt", attempt+1),
				zap.Error(err))

			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxAttempts)
}

func (el *EventListener) Close() {
	el.closeOnce.Do(func() {
		close(el.done)
		el.mu.Lock()
		if el.conn != nil {
			el.conn.Close()
		}
		el.mu.Unlock()
	})
}

func (v *eventValidator) isValidEvent(event Event) bool {
	if !v.validTypes[event.Type] {
		return false
	}

	return event.PoolID != "" && event.TokenA != "" && event.TokenB != ""
}
