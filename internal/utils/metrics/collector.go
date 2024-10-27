// internal/utils/metrics/collector.go
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricType представляет тип метрики
type MetricType string

const (
	TransactionCounterType  MetricType = "transaction_counter"
	TransactionDurationType MetricType = "transaction_duration"
	RPCLatencyType          MetricType = "rpc_latency"
	WebsocketConnectionType MetricType = "websocket_connections"
	PoolLiquidityType       MetricType = "pool_liquidity"
)

// Collector управляет набором метрик
type Collector struct {
	metrics sync.Map
}

// NewCollector создает новый экземпляр коллектора метрик
func NewCollector() *Collector {
	c := &Collector{}
	c.initializeMetrics()
	return c
}

func (c *Collector) initializeMetrics() {
	metricsMap := map[MetricType]prometheus.Collector{
		TransactionCounterType:  transactionCounter,
		TransactionDurationType: transactionDuration,
		RPCLatencyType:          rpcLatency,
		WebsocketConnectionType: websocketConnections,
		PoolLiquidityType:       poolLiquidity,
	}

	for metricType, metric := range metricsMap {
		c.metrics.Store(metricType, metric)
		prometheus.MustRegister(metric)
	}
}

// Reset сбрасывает все метрики (полезно для тестирования)
func (c *Collector) Reset() {
	c.metrics.Range(func(_, value interface{}) bool {
		switch m := value.(type) {
		case *prometheus.CounterVec:
			m.Reset()
		case *prometheus.GaugeVec:
			m.Reset()
		case *prometheus.HistogramVec:
			m.Reset()
		}
		return true
	})
}

// Определение метрик с улучшенными лейблами
var (
	transactionCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "solana_bot",
			Name:      "transactions_total",
			Help:      "Total number of transactions processed",
		},
		[]string{"status", "type", "dex"},
	)

	transactionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "solana_bot",
			Name:      "transaction_duration_seconds",
			Help:      "Transaction duration in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10),
		},
		[]string{"type", "dex"},
	)

	rpcLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "solana_bot",
			Name:      "rpc_latency_seconds",
			Help:      "RPC request latency in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		[]string{"method", "endpoint"},
	)

	websocketConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "solana_bot",
			Name:      "websocket_connections",
			Help:      "Number of active websocket connections",
		},
		[]string{"status"},
	)

	poolLiquidity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "solana_bot",
			Name:      "pool_liquidity",
			Help:      "Current liquidity in pools",
		},
		[]string{"pool_id", "token"},
	)
)
