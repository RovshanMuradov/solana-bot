// pkg/blockchain/solana/rpc_pool.go
package solana

import (
	"sync/atomic"
	"time"
)

// Методы для RPCClient
func (c *RPCClient) setActive(state bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.active = state
}

func (c *RPCClient) isActive() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.active
}

func (c *RPCClient) updateMetrics(success bool, latency time.Duration) {
	c.metrics.mutex.Lock()
	defer c.metrics.mutex.Unlock()

	if success {
		atomic.AddUint64(&c.metrics.successCount, 1)
	} else {
		atomic.AddUint64(&c.metrics.errorCount, 1)
	}

	c.metrics.latency = (c.metrics.latency + latency) / 2 // Скользящее среднее
}

func (c *RPCClient) getMetrics() (uint64, uint64, time.Duration) {
	c.metrics.mutex.RLock()
	defer c.metrics.mutex.RUnlock()
	return c.metrics.successCount, c.metrics.errorCount, c.metrics.latency
}

// Методы для RPCMetrics
func (m *RPCMetrics) reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	atomic.StoreUint64(&m.successCount, 0)
	atomic.StoreUint64(&m.errorCount, 0)
	m.latency = 0
}
