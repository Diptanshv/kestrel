package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Diptanshv/kestrel/internal/httpapi"
)

func TestHealthz(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(httpapi.NewRouter(logger))
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
