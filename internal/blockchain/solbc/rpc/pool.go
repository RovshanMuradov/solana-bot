// internal/blockchain/solbc/rpc/pool.go
package rpc

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// NewPool создает новый пул клиентов
func NewPool(clients []*NodeClient, logger *zap.Logger) *Pool {
	return &Pool{
		Clients:   clients,
		Logger:    logger,
		CurrIndex: 0,
	}
}

// GetNextClient возвращает следующий активный клиент из пула
func (p *Pool) GetNextClient() *NodeClient {
	p.Mutex.Lock()
	defer p.Mutex.Unlock()

	initialIndex := p.CurrIndex
	for {
		p.CurrIndex = (p.CurrIndex + 1) % len(p.Clients)
		if p.Clients[p.CurrIndex].IsActive() {
			return p.Clients[p.CurrIndex]
		}
		if p.CurrIndex == initialIndex {
			return nil
		}
	}
}

// HasActiveClients проверяет наличие активных клиентов в пуле
func (p *Pool) HasActiveClients() bool {
	for _, client := range p.Clients {
		if client.IsActive() {
			return true
		}
	}
	return false
}

// ExecuteWithRetry выполняет операцию с повторными попытками
func (p *Pool) ExecuteWithRetry(ctx context.Context, operation func(*NodeClient) error) error {
	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			client := p.GetNextClient()
			if client == nil {
				return ErrNoActiveClients
			}

			start := time.Now()
			err := operation(client)
			client.UpdateMetrics(err == nil, time.Since(start))

			if err == nil {
				return nil
			}

			lastErr = err
			client.SetActive(false)
			time.Sleep(RetryDelay)
		}
	}

	return lastErr
}
