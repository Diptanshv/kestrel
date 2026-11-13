package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Diptanshv/kestrel/internal/httpapi"
	"github.com/Diptanshv/kestrel/internal/rollup"
	"github.com/Diptanshv/kestrel/internal/store"
)

const rollupSecret = "test-rollup-secret"

func newRollupServer(t *testing.T) (*httptest.Server, *apiClient) {
	t.Helper()
	srv, _ := newTestServer(t) // resets the database
	srv.Close()

	logger := slog.New(slog.DiscardHandler)
	q := store.New(integrationDB)
	loc, _ := time.LoadLocation(integrationCfg.Timezone)
	d := testDeps(logger, q)
	d.Config.RollupSecret = rollupSecret
	d.Rollup = rollup.New(integrationDB, rollup.DefaultConfig(loc), logger)

	srv = httptest.NewServer(httpapi.NewRouter(d))
	t.Cleanup(srv.Close)
	base, _ := url.Parse(srv.URL)
	return srv, newAPIClient(t, srv, base)
}

func triggerRollup(t *testing.T, srv *httptest.Server, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/rollup", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp
}

func TestRollupEndpointAuth(t *testing.T) {
	requireIntegration(t)
	srv, _ := newRollupServer(t)

	if resp := triggerRollup(t, srv, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}
	if resp := triggerRollup(t, srv, "wrong"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}
	if resp := triggerRollup(t, srv, rollupSecret); resp.StatusCode != http.StatusOK {
		t.Errorf("good token: status = %d, want 200", resp.StatusCode)
	}

	// Without a configured secret the route is hidden.
	plain, _ := newTestServer(t)
	if resp := triggerRollup(t, plain, rollupSecret); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unconfigured: status = %d, want 404", resp.StatusCode)
	}
}

// TestHistoryOutlivesRawEvents is the Phase 8 "done when": roll up, delete
// the raw events, and the dashboard still shows the old traffic.
func TestHistoryOutlivesRawEvents(t *testing.T) {
	requireIntegration(t)
	srv, client := newRollupServer(t)
	client.register("hist@example.com", "password123")
	client.login("hist@example.com", "password123")
	_, body := client.postJSON("/api/sites", map[string]string{"domain": "example.com"}, true)
	siteID := siteIDFromCreate(t, body)

	old := time.Now().Add(-4 * 24 * time.Hour)
	visitor := func(b byte) []byte { v := make([]byte, 16); v[0] = b; return v }
	for _, e := range []struct {
		visitor byte
		session int64
		path    string
		offset  time.Duration
	}{
		{1, 11, "/", 0},
		{1, 11, "/pricing", 4 * time.Minute},
		{2, 12, "/", 10 * time.Minute},
	} {
		_, err := integrationDB.Exec(`
			INSERT INTO events_raw (site_id, ts, visitor_id, session_id, name, pathname, country)
			VALUES ($1, $2, $3, $4, 'pageview', $5, 'FR')`,
			siteID, old.Add(e.offset), visitor(e.visitor), e.session, e.path)
		if err != nil {
			t.Fatal(err)
		}
	}

	if resp := triggerRollup(t, srv, rollupSecret); resp.StatusCode != http.StatusOK {
		t.Fatalf("rollup status = %d", resp.StatusCode)
	}
	if n := countEvents(t); n != 0 {
		t.Fatalf("events_raw has %d rows after retention, want 0", n)
	}

	from := url.QueryEscape(time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339))
	path := "/api/sites/" + itoa(siteID)

	resp, body := client.get(path + "/summary?from=" + from)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", resp.StatusCode, body)
	}
	var sum struct {
		Pageviews     int64   `json:"pageviews"`
		Visitors      int64   `json:"visitors"`
		Visits        int64   `json:"visits"`
		BounceRate    float64 `json:"bounce_rate"`
		VisitDuration int64   `json:"visit_duration"`
	}
	_ = json.Unmarshal(body, &sum)
	if sum.Pageviews != 3 || sum.Visitors != 2 || sum.Visits != 2 || sum.BounceRate != 50 || sum.VisitDuration != 120 {
		t.Fatalf("summary = %+v, want 3 pageviews, 2 visitors, 2 visits, 50%% bounce, 120s", sum)
	}

	resp, body = client.get(path + "/breakdown?dimension=page&from=" + from)
	var bd struct {
		Rows []struct {
			Value     string `json:"value"`
			Pageviews int64  `json:"pageviews"`
		} `json:"rows"`
	}
	_ = json.Unmarshal(body, &bd)
	if resp.StatusCode != http.StatusOK || len(bd.Rows) != 2 || bd.Rows[0].Value != "/" || bd.Rows[0].Pageviews != 2 {
		t.Fatalf("page breakdown = %s", body)
	}

	resp, body = client.get(path + "/timeseries?interval=day&from=" + from)
	var ts struct {
		Points []struct {
			Pageviews int64 `json:"pageviews"`
		} `json:"points"`
	}
	_ = json.Unmarshal(body, &ts)
	total := int64(0)
	for _, p := range ts.Points {
		total += p.Pageviews
	}
	if resp.StatusCode != http.StatusOK || total != 3 || len(ts.Points) < 7 {
		t.Fatalf("timeseries total = %d over %d points, body = %s", total, len(ts.Points), body)
	}
}
