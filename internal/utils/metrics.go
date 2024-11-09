// internal/utils/metrics.go
package utils

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	transactionCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "solana_bot_transactions_total",
			Help: "Total number of transactions sent",
		},
		[]string{"status"},
	)
	transactionDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "solana_bot_transaction_duration_seconds",
			Help:    "Duration of transaction preparation and sending",
			Buckets: prometheus.LinearBuckets(0, 0.1, 10),
		},
	)
)

var (
	poolCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "raydium_pool_cache_hits_total",
		Help: "Number of successful pool cache hits",
	})

	poolCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "raydium_pool_cache_misses_total",
		Help: "Number of pool cache misses",
	})

	poolSyncErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "raydium_pool_sync_errors_total",
		Help: "Number of errors during pool synchronization",
	})
)

func init() {
	prometheus.MustRegister(transactionCounter)
	prometheus.MustRegister(transactionDuration)
	prometheus.MustRegister(poolCacheHits)
	prometheus.MustRegister(poolCacheMisses)
	prometheus.MustRegister(poolSyncErrors)
}

func MeasureTransactionDuration(f func() error) error {
	start := time.Now()
	err := f()
	duration := time.Since(start).Seconds()
	transactionDuration.Observe(duration)
	if err != nil {
		transactionCounter.WithLabelValues("failed").Inc()
	} else {
		transactionCounter.WithLabelValues("success").Inc()
	}
	return err
}
