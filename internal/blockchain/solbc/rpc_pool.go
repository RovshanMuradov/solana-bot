// pkg/blockchain/solbc/rpc_pool.go
package solbc

import (
	"sync/atomic"
	"time"
)

// Методы для RPCClient
func (c *RPCNodeClient) setActive(state bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.active = state
}

func (c *RPCNodeClient) isActive() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.active
}

func (c *RPCNodeClient) updateMetrics(success bool, latency time.Duration) {
	c.metrics.mutex.Lock()
	defer c.metrics.mutex.Unlock()

	if success {
		atomic.AddUint64(&c.metrics.successCount, 1)
	} else {
		atomic.AddUint64(&c.metrics.errorCount, 1)
	}

	c.metrics.latency = (c.metrics.latency + latency) / 2 // Скользящее среднее
}

// RPCClient methods
func (c *RPCNodeClient) GetMetrics() (successCount uint64, errorCount uint64, avgLatency time.Duration) {
	c.metrics.mutex.RLock()
	defer c.metrics.mutex.RUnlock()
	return atomic.LoadUint64(&c.metrics.successCount),
		atomic.LoadUint64(&c.metrics.errorCount),
		c.metrics.latency
}

// RPCMetrics methods
func (m *RPCMetrics) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	atomic.StoreUint64(&m.successCount, 0)
	atomic.StoreUint64(&m.errorCount, 0)
	m.latency = 0
}
