package httpapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Diptanshv/kestrel/internal/httpapi"
	"github.com/Diptanshv/kestrel/internal/ratelimit"
	"github.com/Diptanshv/kestrel/internal/store"
)

// newLimitedServer creates a site, then serves it with the given limits.
func newLimitedServer(t *testing.T, limits httpapi.Limits) *httptest.Server {
	t.Helper()
	newSiteForEvents(t, "localhost")
	d := testDeps(slog.New(slog.DiscardHandler), store.New(integrationDB))
	d.Limits = limits
	srv := httptest.NewServer(httpapi.NewRouter(d))
	t.Cleanup(srv.Close)
	return srv
}

var beacon = map[string]any{"d": "localhost", "u": "http://localhost/", "w": 1440}

func TestIngestRateLimitPerIP(t *testing.T) {
	requireIntegration(t)
	srv := newLimitedServer(t, httpapi.Limits{IngestIP: ratelimit.New(0.001, 3)})

	codes := make([]int, 5)
	for i := range codes {
		resp, _ := postEventAs(t, srv, "http://localhost", uaFirefox, beacon)
		codes[i] = resp.StatusCode
	}
	want := []int{202, 202, 202, 429, 429}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("statuses = %v, want %v", codes, want)
		}
	}
	if n := countEvents(t); n != 3 {
		t.Fatalf("stored %d events, want 3", n)
	}
}

func TestIngestRateLimitPerSite(t *testing.T) {
	requireIntegration(t)
	srv := newLimitedServer(t, httpapi.Limits{IngestSite: ratelimit.New(0.001, 2)})
	var last int
	for range 3 {
		resp, _ := postEventAs(t, srv, "http://localhost", uaFirefox, beacon)
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("third event status = %d, want 429", last)
	}
}

func TestIngestRejectsOversizedAndDirtyPayloads(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")

	big := map[string]any{"d": "localhost", "u": "http://localhost/" + strings.Repeat("a", 5000)}
	if resp, _ := postEvent(t, srv, "http://localhost", big); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: status = %d, want 413", resp.StatusCode)
	}

	for _, u := range []string{"http://localhost/%00", "http://localhost/%ff%fe", "http://localhost/a%0Ab"} {
		resp, _ := postEvent(t, srv, "http://localhost", map[string]any{"d": "localhost", "u": u})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("url %q: status = %d, want 400", u, resp.StatusCode)
		}
	}

	// A bad utm_source is dropped rather than failing the event.
	resp, _ := postEvent(t, srv, "http://localhost", map[string]any{"d": "localhost", "u": "http://localhost/?utm_source=%ff"})
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("dirty utm_source: status = %d, want 202", resp.StatusCode)
	}
	if n := countEvents(t); n != 1 {
		t.Fatalf("stored %d events, want 1", n)
	}
}

func TestAuthRateLimit(t *testing.T) {
	requireIntegration(t)
	srv := newLimitedServer(t, httpapi.Limits{Auth: ratelimit.New(0.001, 2)})
	login := func() int {
		resp, err := http.Post(srv.URL+"/api/auth/login", "application/json",
			bytes.NewBufferString(`{"email":"owner@example.com","password":"wrong-password"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	first, second, third := login(), login(), login()
	if first == http.StatusTooManyRequests || second == http.StatusTooManyRequests || third != http.StatusTooManyRequests {
		t.Fatalf("login statuses = %d %d %d, want the third throttled", first, second, third)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	c := newAPIClient(t, srv, base)
	c.register("flags@example.com", "password123")
	resp, _ := c.postJSON("/api/auth/login", map[string]string{"email": "flags@example.com", "password": "password123"}, false)
	var sawSession bool
	for _, ck := range resp.Cookies() {
		if ck.Name == "csrf_token" {
			if ck.HttpOnly {
				t.Error("csrf cookie must be readable by the dashboard")
			}
			continue
		}
		sawSession = true
		if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
			t.Errorf("session cookie %q: HttpOnly=%v SameSite=%v, want HttpOnly and Lax", ck.Name, ck.HttpOnly, ck.SameSite)
		}
	}
	if !sawSession {
		t.Fatal("login set no session cookie")
	}
}
