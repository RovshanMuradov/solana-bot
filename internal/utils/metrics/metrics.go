// internal/utils/metrics/metrics.go
package metrics

import (
	"time"
)

// RecordRPCLatency записывает метрики RPC-запроса
func (c *Collector) RecordRPCLatency(method, endpoint string, duration time.Duration) {
	rpcLatency.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

// UpdateWebsocketConnections обновляет метрики веб-сокет соединений
func (c *Collector) UpdateWebsocketConnections(active int, status string) {
	websocketConnections.WithLabelValues(status).Set(float64(active))
}

// UpdatePoolLiquidity обновляет метрики пула
func (c *Collector) UpdatePoolLiquidity(poolID, token string, amount float64) {
	poolLiquidity.WithLabelValues(poolID, token).Set(amount)
}
