package httpapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/Diptanshv/kestrel/internal/httpapi"
	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/Diptanshv/kestrel/internal/testdb"
)

const csrfHeader = "X-CSRF-Token"

var (
	integrationDB      *sql.DB
	integrationCfg     config.Config
	integrationEnabled bool
)

func TestMain(m *testing.M) {
	db, cleanup, err := testdb.Setup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests skipped: %v\n", err)
		integrationEnabled = false
		os.Exit(m.Run())
	}
	integrationDB = db
	integrationCfg = testdb.TestConfig()
	integrationEnabled = true
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func requireIntegration(t *testing.T) {
	if !integrationEnabled {
		t.Skip("integration database not available (start docker-compose or set TEST_DATABASE_URL)")
	}
}

type apiClient struct {
	t      *testing.T
	server *httptest.Server
	client *http.Client
	base   *url.URL
}

func newTestServer(t *testing.T) (*httptest.Server, *url.URL) {
	t.Helper()
	requireIntegration(t)
	testdb.Reset(t, integrationDB)
	logger := slog.New(slog.DiscardHandler)
	q := store.New(integrationDB)
	srv := httptest.NewServer(httpapi.NewRouter(logger, q, integrationCfg))
	base, err := url.Parse(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("parse server url: %v", err)
	}
	t.Cleanup(srv.Close)
	return srv, base
}

func newAPIClient(t *testing.T, srv *httptest.Server, base *url.URL) *apiClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &apiClient{
		t:      t,
		server: srv,
		client: &http.Client{Jar: jar},
		base:   base,
	}
}

func (c *apiClient) csrfToken() string {
	for _, cookie := range c.client.Jar.Cookies(c.base) {
		if cookie.Name == "csrf_token" && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

func (c *apiClient) do(req *http.Request) (*http.Response, []byte) {
	resp, err := c.client.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		c.t.Fatalf("read body: %v", err)
	}
	return resp, body
}

func (c *apiClient) postJSON(path string, payload any, withCSRF bool) (*http.Response, []byte) {
	b, err := json.Marshal(payload)
	if err != nil {
		c.t.Fatalf("marshal json: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.server.URL+path, bytes.NewReader(b))
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if withCSRF {
		req.Header.Set(csrfHeader, c.csrfToken())
	}
	return c.do(req)
}

func (c *apiClient) get(path string) (*http.Response, []byte) {
	req, err := http.NewRequest(http.MethodGet, c.server.URL+path, nil)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	return c.do(req)
}

func (c *apiClient) patchJSON(path string, payload any) (*http.Response, []byte) {
	b, err := json.Marshal(payload)
	if err != nil {
		c.t.Fatalf("marshal json: %v", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.server.URL+path, bytes.NewReader(b))
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, c.csrfToken())
	return c.do(req)
}

func (c *apiClient) delete(path string) (*http.Response, []byte) {
	req, err := http.NewRequest(http.MethodDelete, c.server.URL+path, nil)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set(csrfHeader, c.csrfToken())
	return c.do(req)
}

func (c *apiClient) register(email, password string) {
	resp, _ := c.postJSON("/api/auth/register", map[string]string{
		"email":    email,
		"password": password,
	}, false)
	if resp.StatusCode != http.StatusCreated {
		c.t.Fatalf("register status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func (c *apiClient) login(email, password string) {
	resp, _ := c.postJSON("/api/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, false)
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if c.csrfToken() == "" {
		c.t.Fatal("login did not set csrf cookie")
	}
}

func TestAuthMeRequiresSession(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)

	resp, _ := client.get("/api/auth/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuthRegisterLoginMeLogout(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)

	const email = "user@example.com"
	const password = "password123"

	client.register(email, password)
	client.login(email, password)

	resp, body := client.get("/api/auth/me")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want %d, body = %s", resp.StatusCode, http.StatusOK, body)
	}
	var me struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.Email != email {
		t.Fatalf("me email = %q, want %q", me.Email, email)
	}

	resp, _ = client.postJSON("/api/auth/logout", map[string]string{}, true)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	resp, _ = client.get("/api/auth/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSiteOwnership(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	owner := newAPIClient(t, srv, base)
	other := newAPIClient(t, srv, base)

	owner.register("owner@example.com", "password123")
	owner.login("owner@example.com", "password123")

	resp, body := owner.postJSON("/api/sites", map[string]string{"domain": "owned.example.com"}, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create site status = %d, want %d, body = %s", resp.StatusCode, http.StatusCreated, body)
	}
	var site struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &site); err != nil {
		t.Fatalf("decode site: %v", err)
	}

	other.register("other@example.com", "password123")
	other.login("other@example.com", "password123")

	path := "/api/sites/" + strconv.FormatInt(site.ID, 10)
	for _, tc := range []struct {
		name string
		fn   func() (*http.Response, []byte)
	}{
		{"GET", func() (*http.Response, []byte) { return other.get(path) }},
		{"PATCH", func() (*http.Response, []byte) {
			return other.patchJSON(path, map[string]string{"domain": "stolen.example.com"})
		}},
		{"DELETE", func() (*http.Response, []byte) { return other.delete(path) }},
	} {
		resp, _ := tc.fn()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s other user status = %d, want %d", tc.name, resp.StatusCode, http.StatusForbidden)
		}
	}

	resp, _ = owner.get(path)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner GET status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
