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
	get := func(cfg config.Config, token string) (*http.Response, string) {
		t.Helper()
		srv := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{Log: logger, Config: cfg, Enrich: enricher}))
		defer srv.Close()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/metrics", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /metrics: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	if resp, _ := get(config.Config{}, "anything"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("no token configured: status = %d, want 404", resp.StatusCode)
	}
	withToken := config.Config{MetricsToken: "m-secret"}
	if resp, _ := get(withToken, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing token: status = %d, want 401", resp.StatusCode)
	}
	if resp, _ := get(withToken, "wrong"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}
	resp, body := get(withToken, "m-secret")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("good token: status = %d", resp.StatusCode)
	}
	for _, name := range []string{"events_received_total", "batch_flush_duration_seconds", "buffer_depth"} {
		if !strings.Contains(body, name) {
			t.Errorf("/metrics missing %s", name)
		}
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}
