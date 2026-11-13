package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Diptanshv/kestrel/internal/batch"
	"github.com/Diptanshv/kestrel/internal/httpapi"
	"github.com/Diptanshv/kestrel/internal/store"
)

// TestIngestLatency drives POST /api/event concurrently with the synchronous
// sink and with the batcher, and reports latency percentiles. It is a
// measurement, not a pass/fail check, so it only runs when asked:
//
//	KESTREL_BENCH=1 go test ./internal/httpapi -run TestIngestLatency -v
//
// Numbers from this go into docs/benchmarks.md.
func TestIngestLatency(t *testing.T) {
	if os.Getenv("KESTREL_BENCH") == "" {
		t.Skip("set KESTREL_BENCH=1 to run")
	}
	requireIntegration(t)

	const (
		workers  = 32
		requests = 8000
	)
	logger := slog.New(slog.DiscardHandler)
	q := store.New(integrationDB)

	run := func(name string, sink httpapi.EventSink, done func()) {
		newSiteForEvents(t, "localhost") // resets the DB and creates the site
		d := testDeps(logger, q)
		d.Sink = sink
		srv := httptest.NewServer(httpapi.NewRouter(d))
		defer srv.Close()

		body := []byte(`{"d":"localhost","n":"pageview","u":"http://localhost/pricing","w":1440}`)
		client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: workers}}
		lat := make([]time.Duration, requests)

		var wg sync.WaitGroup
		start := time.Now()
		for w := range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := w; i < requests; i += workers {
					req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/event", bytes.NewReader(body))
					req.Header.Set("Content-Type", "text/plain")
					req.Header.Set("Origin", "http://localhost")
					// Distinct UAs so visitor hashing does real work.
					req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Firefox/120."+strconv.Itoa(i))
					t0 := time.Now()
					resp, err := client.Do(req)
					lat[i] = time.Since(t0)
					if err != nil {
						t.Error(err)
						return
					}
					_ = resp.Body.Close()
					if resp.StatusCode != http.StatusAccepted {
						t.Errorf("status %d", resp.StatusCode)
						return
					}
				}
			}()
		}
		wg.Wait()
		elapsed := time.Since(start)
		done()

		slices.Sort(lat)
		pct := func(p float64) time.Duration { return lat[int(float64(len(lat)-1)*p)] }
		t.Logf("%-8s %6d req  %7.0f req/s  p50=%-9s p95=%-9s p99=%-9s max=%s  rows=%d",
			name, requests, float64(requests)/elapsed.Seconds(),
			pct(0.50).Round(time.Microsecond), pct(0.95).Round(time.Microsecond),
			pct(0.99).Round(time.Microsecond), lat[len(lat)-1].Round(time.Microsecond),
			countEvents(t))
	}

	run("sync", batch.Direct{W: batch.InsertWriter{Q: q}, Log: logger}, func() {})

	b := batch.New(batch.CopyWriter{DB: integrationDB}, logger, batch.Config{})
	b.Start()
	run("batched", b, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := b.Close(ctx); err != nil {
			t.Fatal(fmt.Errorf("drain: %w", err))
		}
	})
}
