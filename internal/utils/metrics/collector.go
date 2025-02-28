// internal/utils/metrics/collector.go
package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
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

// Collector управляет набором метрик и содержит ссылки на необходимые объекты
type Collector struct {
	metrics    sync.Map
	clientLock sync.RWMutex   // Mutex for thread-safe access to the Solana client
	walletLock sync.RWMutex   // Mutex for thread-safe access to the wallet
	solClient  *solbc.Client  // Reference to Solana client
	userWallet *wallet.Wallet // Reference to user wallet
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

// SetSolanaClient stores a reference to the Solana client
func (c *Collector) SetSolanaClient(client *solbc.Client) {
	c.clientLock.Lock()
	defer c.clientLock.Unlock()
	c.solClient = client
}

// GetSolanaClient returns the stored Solana client reference
func (c *Collector) GetSolanaClient() (*solbc.Client, bool) {
	c.clientLock.RLock()
	defer c.clientLock.RUnlock()

	if c.solClient == nil {
		return nil, false
	}
	return c.solClient, true
}

// SetDefaultWallet stores a reference to the user wallet
func (c *Collector) SetDefaultWallet(w *wallet.Wallet) {
	c.walletLock.Lock()
	defer c.walletLock.Unlock()
	c.userWallet = w
}

// GetUserWallet returns the stored user wallet reference
func (c *Collector) GetUserWallet() (*wallet.Wallet, error) {
	c.walletLock.RLock()
	defer c.walletLock.RUnlock()

	if c.userWallet == nil {
		return nil, fmt.Errorf("user wallet not set")
	}
	return c.userWallet, nil
}

// RecordTransaction records transaction metrics
func (c *Collector) RecordTransaction(ctx context.Context, txType, dexName string, duration time.Duration, success bool) {
	// Get counter from metrics map
	counter, ok := c.metrics.Load(TransactionCounterType)
	if !ok {
		return
	}

	// Record transaction count
	status := "success"
	if !success {
		status = "failure"
	}

	if counterVec, ok := counter.(*prometheus.CounterVec); ok {
		counterVec.WithLabelValues(status, txType, dexName).Inc()
	}

	// Record duration
	if durationMetric, ok := c.metrics.Load(TransactionDurationType); ok {
		if histVec, ok := durationMetric.(*prometheus.HistogramVec); ok {
			histVec.WithLabelValues(txType, dexName).Observe(duration.Seconds())
		}
	}
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
