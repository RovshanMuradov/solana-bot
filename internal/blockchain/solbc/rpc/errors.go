// internal/blockchain/solbc/rpc/errors.go
package rpc

import (
	"errors"
	"fmt"
)

var (
	// ErrNoActiveClients возникает, когда нет доступных активных клиентов
	ErrNoActiveClients = errors.New("no active RPC clients available")

	// ErrRateLimit возникает при превышении лимита запросов
	ErrRateLimit = errors.New("rate limit exceeded")

	// ErrTimeout возникает при превышении времени ожидания
	ErrTimeout = errors.New("request timeout")

	// ErrInvalidResponse возникает при получении некорректного ответа
	ErrInvalidResponse = errors.New("invalid RPC response")

	// ErrConnectionFailed возникает при ошибке подключения
	ErrConnectionFailed = errors.New("connection failed")
)

// Error представляет ошибку RPC с дополнительным контекстом
type Error struct {
	Err     error
	NodeURL string
	Method  string
}

// Error реализует интерфейс error
func (e *Error) Error() string {
	return fmt.Sprintf("RPC error [%s] at %s: %v", e.Method, e.NodeURL, e.Err)
}

// Unwrap возвращает оригинальную ошибку
func (e *Error) Unwrap() error {
	return e.Err
}

// NewError создает новую ошибку RPC
func NewError(err error, nodeURL, method string) error {
	return &Error{
		Err:     err,
		NodeURL: nodeURL,
		Method:  method,
	}
}
