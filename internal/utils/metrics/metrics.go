// internal/utils/metrics/metrics.go
package metrics

import (
	"context"
	"time"
)

// RecordTransaction записывает метрики транзакции с учетом контекста
func (c *Collector) RecordTransaction(ctx context.Context, txType, dex string, duration time.Duration, success bool) {
	// Проверяем, не отменен ли контекст
	select {
	case <-ctx.Done():
		// Если контекст отменен, записываем метрику с пометкой cancelled
		transactionCounter.WithLabelValues("cancelled", txType, dex).Inc()
		return
	default:
		// Продолжаем обычное выполнение
		status := "success"
		if !success {
			status = "failed"
		}

		// Записываем метрики транзакции
		transactionCounter.WithLabelValues(status, txType, dex).Inc()
		transactionDuration.WithLabelValues(txType, dex).Observe(duration.Seconds())
	}
}

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
