// internal/blockchain/solbc/rpc/client.go
package rpc

import (
	"sync/atomic"
	"time"

	solanarpc "github.com/gagliardetto/solana-go/rpc"
)

// NewClient создает новый экземпляр NodeClient
func NewClient(url string) (*NodeClient, error) {
	return &NodeClient{
		Client:  solanarpc.New(url),
		URL:     url,
		active:  true,
		metrics: newMetrics(),
	}, nil
}

// newMetrics создает новый экземпляр метрик
func newMetrics() *metrics {
	return &metrics{
		successCount: 0,
		errorCount:   0,
		latency:      0,
	}
}

// GetMetrics возвращает текущие метрики узла
func (c *NodeClient) GetMetrics() (uint64, uint64, time.Duration) {
	c.metrics.mutex.RLock()
	defer c.metrics.mutex.RUnlock()

	return atomic.LoadUint64(&c.metrics.successCount),
		atomic.LoadUint64(&c.metrics.errorCount),
		c.metrics.latency
}

// SetActive устанавливает статус активности узла
func (c *NodeClient) SetActive(state bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.active = state
}

// IsActive возвращает текущий статус активности узла
func (c *NodeClient) IsActive() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.active
}

// UpdateMetrics обновляет метрики узла
func (c *NodeClient) UpdateMetrics(success bool, latency time.Duration) {
	c.metrics.mutex.Lock()
	defer c.metrics.mutex.Unlock()

	if success {
		atomic.AddUint64(&c.metrics.successCount, 1)
	} else {
		atomic.AddUint64(&c.metrics.errorCount, 1)
	}

	c.metrics.latency = (c.metrics.latency + latency) / 2
}
