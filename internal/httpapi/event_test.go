package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Diptanshv/kestrel/internal/httpapi"
	"github.com/Diptanshv/kestrel/internal/store"
)

// postEvent sends a tracker-shaped beacon: text/plain body, explicit Origin,
// no session cookie and no CSRF token.
func postEvent(t *testing.T, srv *httptest.Server, origin string, payload any) (*http.Response, []byte) {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/event", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}

func postEventAs(t *testing.T, srv *httptest.Server, origin, userAgent string, payload any) (*http.Response, []byte) {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/event", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}

// newSiteForEvents registers a user, logs in, and creates one site.
func newSiteForEvents(t *testing.T, domain string) *httptest.Server {
	t.Helper()
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)
	client.register("owner@example.com", "password123")
	client.login("owner@example.com", "password123")

	resp, body := client.postJSON("/api/sites", map[string]string{"domain": domain}, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create site status = %d, want %d, body = %s", resp.StatusCode, http.StatusCreated, body)
	}
	return srv
}

// newSiteForEventsOnExistingDB starts a server without resetting the database.
func newSiteForEventsOnExistingDB(t *testing.T) *httptest.Server {
	t.Helper()
	requireIntegration(t)
	logger := slog.New(slog.DiscardHandler)
	q := store.New(integrationDB)
	srv := httptest.NewServer(httpapi.NewRouter(testDeps(logger, q)))
	t.Cleanup(srv.Close)
	return srv
}

func countEvents(t *testing.T) int {
	t.Helper()
	var n int
	if err := integrationDB.QueryRow("SELECT count(*) FROM events_raw").Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func TestEventValidPageview(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	resp, body := postEvent(t, srv, "http://localhost:8080", map[string]any{
		"d": "localhost",
		"n": "pageview",
		"u": "http://localhost:8080/pricing/?utm_source=HN",
		"r": "https://news.ycombinator.com/item?id=1",
		"w": 1440,
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, http.StatusAccepted, body)
	}

	var (
		name, pathname string
		referrer       string
		utmSource      string
		device         string
		visitorLen     int
	)
	err := integrationDB.QueryRow(`
		SELECT name, pathname, referrer, utm_source, device, octet_length(visitor_id)
		FROM events_raw ORDER BY id DESC LIMIT 1
	`).Scan(&name, &pathname, &referrer, &utmSource, &device, &visitorLen)
	if err != nil {
		t.Fatalf("read event row: %v", err)
	}

	if name != "pageview" {
		t.Errorf("name = %q, want %q", name, "pageview")
	}
	if pathname != "/pricing" {
		t.Errorf("pathname = %q, want %q", pathname, "/pricing")
	}
	if referrer != "news.ycombinator.com" {
		t.Errorf("referrer = %q, want %q", referrer, "news.ycombinator.com")
	}
	if utmSource != "hn" {
		t.Errorf("utm_source = %q, want %q", utmSource, "hn")
	}
	if device != "desktop" {
		t.Errorf("device = %q, want %q", device, "desktop")
	}
	if visitorLen != 16 {
		t.Errorf("octet_length(visitor_id) = %d, want 16", visitorLen)
	}
}

func TestEventInvalidPayload(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"missing url", map[string]any{"d": "localhost"}},
		{"missing domain", map[string]any{"u": "http://localhost:8080/"}},
		{"bad scheme", map[string]any{"d": "localhost", "u": "file:///etc/passwd"}},
		{"bad event name", map[string]any{"d": "localhost", "n": "sign up", "u": "http://localhost:8080/"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := postEvent(t, srv, "http://localhost:8080", tc.payload)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, http.StatusBadRequest, body)
			}
		})
	}

	if n := countEvents(t); n != 0 {
		t.Fatalf("events_raw has %d rows, want 0", n)
	}
}

func TestEventMismatchedOrigin(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	payload := map[string]any{"d": "localhost", "u": "http://localhost:8080/", "w": 1440}

	for _, origin := range []string{"https://evil.com", ""} {
		resp, body := postEvent(t, srv, origin, payload)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q: status = %d, want %d, body = %s", origin, resp.StatusCode, http.StatusForbidden, body)
		}
	}

	if n := countEvents(t); n != 0 {
		t.Fatalf("events_raw has %d rows, want 0", n)
	}
}

func TestEventUnknownSite(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	resp, body := postEvent(t, srv, "https://unregistered.com", map[string]any{
		"d": "unregistered.com",
		"u": "https://unregistered.com/",
		"w": 1440,
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, http.StatusNotFound, body)
	}
}

func TestEventBotIsAcceptedAndDropped(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	req := map[string]any{"d": "localhost", "u": "http://localhost:8080/", "w": 1440}
	resp, body := postEventAs(t, srv, "http://localhost:8080",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", req)

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, http.StatusAccepted, body)
	}
	if n := countEvents(t); n != 0 {
		t.Fatalf("events_raw has %d rows, want 0", n)
	}
}

func TestEventRecordsBrowserAndDevice(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	const chrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	resp, body := postEventAs(t, srv, "http://localhost:8080", chrome,
		map[string]any{"d": "localhost", "u": "http://localhost:8080/", "w": 1440})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var browser, os, device string
	err := integrationDB.QueryRow(
		`SELECT browser, os, device FROM events_raw ORDER BY id DESC LIMIT 1`,
	).Scan(&browser, &os, &device)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if browser != "Chrome" {
		t.Errorf("browser = %q, want Chrome", browser)
	}
	if device != "desktop" {
		t.Errorf("device = %q, want desktop", device)
	}
	if os == "" {
		t.Error("os is empty")
	}
}

func TestSaltRotationChangesVisitorIDs(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	const ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	body := map[string]any{"d": "localhost", "u": "http://localhost:8080/", "w": 1440}

	postEventAs(t, srv, "http://localhost:8080", ua, body)

	if _, err := integrationDB.Exec(`DELETE FROM salts`); err != nil {
		t.Fatalf("clear salts: %v", err)
	}
	srv2 := newSiteForEventsOnExistingDB(t)
	postEventAs(t, srv2, "http://localhost:8080", ua, body)

	var distinct int
	if err := integrationDB.QueryRow(
		`SELECT count(DISTINCT visitor_id) FROM events_raw`,
	).Scan(&distinct); err != nil {
		t.Fatalf("count: %v", err)
	}
	if distinct != 2 {
		t.Fatalf("distinct visitor_id = %d, want 2 (a new salt must break linkage)", distinct)
	}
}

func TestScriptJS(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)

	resp, body := client.get("/script.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Cache-Control"); got == "" {
		t.Error("Cache-Control header is empty")
	}
	if !bytes.Contains(body, []byte("sendBeacon")) {
		t.Error("script.js does not contain sendBeacon")
	}
}
