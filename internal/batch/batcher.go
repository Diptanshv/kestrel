package batch

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Diptanshv/kestrel/internal/metrics"
)

// Config sizes the pipeline. Defaults match the design doc: a 10k buffer
// absorbs bursts, and a flush happens at 500 events or every 2 seconds,
// whichever comes first.
type Config struct {
	BufferSize    int
	BatchSize     int
	FlushInterval time.Duration
	// WriteTimeout bounds a single flush so a stuck database cannot wedge the
	// batcher forever.
	WriteTimeout time.Duration
}

func (c Config) withDefaults() Config {
	if c.BufferSize <= 0 {
		c.BufferSize = 10_000
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 500
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = 2 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 10 * time.Second
	}
	return c
}

// Batcher owns the ingest channel and the single goroutine that drains it.
type Batcher struct {
	w   Writer
	log *slog.Logger
	cfg Config

	ch   chan Event
	done chan struct{}

	// mu guards closed. Enqueue holds the read lock across its send so Close
	// cannot close the channel underneath it.
	mu     sync.RWMutex
	closed bool
}

func New(w Writer, log *slog.Logger, cfg Config) *Batcher {
	cfg = cfg.withDefaults()
	return &Batcher{
		w:    w,
		log:  log,
		cfg:  cfg,
		ch:   make(chan Event, cfg.BufferSize),
		done: make(chan struct{}),
	}
}

// Start launches the flush loop. Call it once.
func (b *Batcher) Start() {
	go b.run()
}

// Enqueue hands an event to the pipeline without blocking. It returns false
// when the event was dropped, either because the buffer is full or because
// the batcher is shutting down. Losing analytics under overload is better
// than slowing the customer's page.
func (b *Batcher) Enqueue(ev Event) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		metrics.EventsDropped.WithLabelValues(metrics.DropShutdown).Inc()
		return false
	}
	select {
	case b.ch <- ev:
		metrics.EventsReceived.Inc()
		return true
	default:
		metrics.EventsDropped.WithLabelValues(metrics.DropBufferFull).Inc()
		return false
	}
}

// Close stops accepting events, drains what is buffered, flushes, and waits
// for the loop to exit or ctx to expire. Call it after the HTTP server has
// stopped so no handler is mid-Enqueue.
func (b *Batcher) Close(ctx context.Context) error {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.ch)
	}
	b.mu.Unlock()

	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Depth reports how many events are waiting in the channel.
func (b *Batcher) Depth() int { return len(b.ch) }

func (b *Batcher) run() {
	defer close(b.done)

	ticker := time.NewTicker(b.cfg.FlushInterval)
	defer ticker.Stop()

	buf := make([]Event, 0, b.cfg.BatchSize)
	for {
		select {
		case ev, ok := <-b.ch:
			if !ok {
				b.flush(buf)
				return
			}
			buf = append(buf, ev)
			if len(buf) >= b.cfg.BatchSize {
				b.flush(buf)
				buf = buf[:0]
			}
		case <-ticker.C:
			b.flush(buf)
			buf = buf[:0]
		}
	}
}

func (b *Batcher) flush(buf []Event) {
	metrics.BufferDepth.Set(float64(len(b.ch)))
	if len(buf) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.WriteTimeout)
	defer cancel()

	start := time.Now()
	err := b.w.Write(ctx, buf)
	metrics.BatchFlushDuration.Observe(time.Since(start).Seconds())
	metrics.BatchSize.Observe(float64(len(buf)))

	if err != nil {
		// No retry: a failed COPY is all-or-nothing, and holding the batch
		// would let one bad row or an outage back the whole pipeline up.
		metrics.EventsDropped.WithLabelValues(metrics.DropWriteError).Add(float64(len(buf)))
		b.log.Error("batch flush failed", "events", len(buf), "err", err)
		return
	}
	metrics.EventsWritten.Add(float64(len(buf)))
}

// Direct writes each event synchronously on the caller's goroutine. It is the
// naive path the batcher replaced, kept for tests that assert on rows right
// after a request and as the benchmark baseline.
type Direct struct {
	W   Writer
	Log *slog.Logger
}

func (d Direct) Enqueue(ev Event) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.W.Write(ctx, []Event{ev}); err != nil {
		if d.Log != nil {
			d.Log.Error("direct write failed", "err", err)
		}
		metrics.EventsDropped.WithLabelValues(metrics.DropWriteError).Inc()
		return false
	}
	metrics.EventsReceived.Inc()
	metrics.EventsWritten.Inc()
	return true
}
