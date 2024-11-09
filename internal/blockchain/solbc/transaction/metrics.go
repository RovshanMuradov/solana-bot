// internal/blockchain/solbc/transaction/metrics.go
package transaction

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	successCounter    prometheus.Counter
	failureCounter    prometheus.Counter
	durationHistogram prometheus.Histogram
}

func NewMetrics() *Metrics {
	successCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "solana_tx_success_total",
		Help: "Total number of successful transactions",
	})
	failureCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "solana_tx_failure_total",
		Help: "Total number of failed transactions",
	})
	durationHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "solana_tx_duration_seconds",
		Help:    "Transaction duration in seconds",
		Buckets: prometheus.LinearBuckets(0, 0.1, 10),
	})

	prometheus.MustRegister(successCounter, failureCounter, durationHistogram)

	return &Metrics{
		successCounter:    successCounter,
		failureCounter:    failureCounter,
		durationHistogram: durationHistogram,
	}
}

func (tm *Metrics) TrackTransaction(start time.Time) {
	tm.durationHistogram.Observe(time.Since(start).Seconds())
}
