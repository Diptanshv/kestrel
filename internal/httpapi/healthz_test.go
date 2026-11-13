package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/Diptanshv/kestrel/internal/enrich"
	"github.com/Diptanshv/kestrel/internal/httpapi"
)

func TestHealthz(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	enricher := enrich.New(enrich.NewSaltStore(nil), &enrich.Geo{})
	srv := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{Log: logger, Config: config.Config{}, Enrich: enricher}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	const want = `{"status":"ok"}` + "\n"
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	enricher := enrich.New(enrich.NewSaltStore(nil), &enrich.Geo{})
	srv := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{Log: logger, Enrich: enricher}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	for _, name := range []string{"events_received_total", "batch_flush_duration_seconds", "buffer_depth"} {
		if !strings.Contains(string(body), name) {
			t.Errorf("/metrics missing %s", name)
		}
	}
}
