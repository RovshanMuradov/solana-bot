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

func init() {
	prometheus.MustRegister(transactionCounter)
	prometheus.MustRegister(transactionDuration)
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
