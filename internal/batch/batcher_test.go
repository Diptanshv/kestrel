package batch

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// memWriter records every batch it receives. block, when set, holds Write
// until it is closed so tests can fill the channel deterministically.
type memWriter struct {
	mu      sync.Mutex
	batches [][]Event
	block   chan struct{}
	err     error
}

func (w *memWriter) Write(_ context.Context, events []Event) error {
	if w.block != nil {
		<-w.block
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	cp := make([]Event, len(events))
	copy(cp, events)
	w.batches = append(w.batches, cp)
	return nil
}

func (w *memWriter) total() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, b := range w.batches {
		n += len(b)
	}
	return n
}

func (w *memWriter) sizes() []int {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]int, len(w.batches))
	for i, b := range w.batches {
		out[i] = len(b)
	}
	return out
}

func ev(i int) Event { return Event{SiteID: int64(i)} }

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}

func TestFlushOnBatchSize(t *testing.T) {
	w := &memWriter{}
	b := New(w, discard(), Config{BatchSize: 10, FlushInterval: time.Hour})
	b.Start()
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	for i := range 25 {
		if !b.Enqueue(ev(i)) {
			t.Fatalf("enqueue %d dropped", i)
		}
	}
	// Two full batches flush on size; the remaining 5 wait for the timer.
	waitFor(t, func() bool { return w.total() == 20 })
	if got := w.sizes(); len(got) != 2 || got[0] != 10 || got[1] != 10 {
		t.Fatalf("batch sizes = %v, want [10 10]", got)
	}
}

func TestFlushOnInterval(t *testing.T) {
	w := &memWriter{}
	b := New(w, discard(), Config{BatchSize: 1000, FlushInterval: 20 * time.Millisecond})
	b.Start()
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	for i := range 3 {
		b.Enqueue(ev(i))
	}
	waitFor(t, func() bool { return w.total() == 3 })
}

func TestEnqueueDropsWhenFull(t *testing.T) {
	w := &memWriter{block: make(chan struct{})}
	b := New(w, discard(), Config{BufferSize: 4, BatchSize: 1, FlushInterval: time.Hour})
	b.Start()

	// The first event is pulled off the channel and blocks in Write; the
	// next four fill the buffer; everything after that must be dropped.
	b.Enqueue(ev(0))
	waitFor(t, func() bool { return b.Depth() == 0 })
	accepted := 0
	for i := 1; i <= 10; i++ {
		if b.Enqueue(ev(i)) {
			accepted++
		}
	}
	if accepted != 4 {
		t.Fatalf("accepted = %d, want 4 (buffer size)", accepted)
	}

	close(w.block)
	if err := b.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := w.total(); got != 5 {
		t.Fatalf("written = %d, want 5", got)
	}
}

func TestCloseDrainsEverything(t *testing.T) {
	w := &memWriter{}
	b := New(w, discard(), Config{BatchSize: 64, FlushInterval: time.Hour})
	b.Start()

	const n = 1000
	var wg sync.WaitGroup
	for g := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range n / 10 {
				if !b.Enqueue(ev(g*1000 + i)) {
					t.Errorf("event dropped before close")
				}
			}
		}()
	}
	wg.Wait()

	if err := b.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := w.total(); got != n {
		t.Fatalf("written = %d, want %d: graceful stop lost events", got, n)
	}
}

func TestEnqueueAfterCloseIsRejected(t *testing.T) {
	b := New(&memWriter{}, discard(), Config{})
	b.Start()
	if err := b.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.Enqueue(ev(1)) {
		t.Fatal("enqueue after close returned true")
	}
	// Close is idempotent.
	if err := b.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCloseRespectsContext(t *testing.T) {
	w := &memWriter{block: make(chan struct{})}
	defer close(w.block)
	b := New(w, discard(), Config{BatchSize: 1})
	b.Start()
	b.Enqueue(ev(1))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close err = %v, want deadline exceeded", err)
	}
}

func TestWriteErrorDropsBatchAndContinues(t *testing.T) {
	w := &memWriter{err: errors.New("db down")}
	b := New(w, discard(), Config{BatchSize: 2, FlushInterval: time.Hour})
	b.Start()
	b.Enqueue(ev(1))
	b.Enqueue(ev(2))

	waitFor(t, func() bool { return b.Depth() == 0 })
	w.mu.Lock()
	w.err = nil
	w.mu.Unlock()

	b.Enqueue(ev(3))
	if err := b.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := w.total(); got != 1 {
		t.Fatalf("written = %d, want 1 (failed batch is dropped, loop keeps going)", got)
	}
}
