// Package metrics holds the process-wide Prometheus collectors. They are
// package-level so any package can record without threading a registry
// through constructors; /metrics serves the default registry.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Drop reasons for EventsDropped. Kept as constants so dashboards and alerts
// do not drift from the code.
const (
	DropBufferFull = "buffer_full"
	DropShutdown   = "shutdown"
	DropWriteError = "write_error"
	DropBot        = "bot"
	DropRateLimit  = "rate_limited"
)

var (
	EventsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "events_received_total",
		Help: "Events accepted by the ingest handler and handed to the pipeline.",
	})

	EventsDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "events_dropped_total",
		Help: "Events discarded before reaching the database, by reason.",
	}, []string{"reason"})

	EventsWritten = promauto.NewCounter(prometheus.CounterOpts{
		Name: "events_written_total",
		Help: "Events persisted to events_raw.",
	})

	BatchFlushDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "batch_flush_duration_seconds",
		Help:    "Time to write one batch to Postgres.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms .. ~2s
	})

	BatchSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "batch_size",
		Help:    "Events per flushed batch.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
	})

	BufferDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "buffer_depth",
		Help: "Events waiting in the ingest channel at the last observation.",
	})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by route pattern.",
		Buckets: prometheus.ExponentialBuckets(0.0005, 2, 14), // 0.5ms .. ~4s
	}, []string{"method", "route", "status"})
)
