package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

const (
	uaFirefox = "Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0"
	uaChrome  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// TestIngestAssignsSessions checks the live path: one visitor's events share
// a session, a second visitor gets their own.
func TestIngestAssignsSessions(t *testing.T) {
	requireIntegration(t)
	srv := newSiteForEvents(t, "localhost")
	pv := map[string]any{"d": "localhost", "u": "http://localhost/", "w": 1440}

	for _, ua := range []string{uaFirefox, uaFirefox, uaChrome, uaFirefox} {
		if resp, body := postEventAs(t, srv, "http://localhost", ua, pv); resp.StatusCode != http.StatusAccepted {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
	}

	rows, err := integrationDB.Query(`SELECT session_id, count(*) FROM events_raw GROUP BY session_id ORDER BY count(*)`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var counts []int
	for rows.Next() {
		var sid int64
		var n int
		if err := rows.Scan(&sid, &n); err != nil {
			t.Fatal(err)
		}
		if sid == 0 {
			t.Fatal("event stored without a session")
		}
		counts = append(counts, n)
	}
	if len(counts) != 2 || counts[0] != 1 || counts[1] != 3 {
		t.Fatalf("events per session = %v, want [1 3]", counts)
	}
}

func TestSummaryVisitsBounceDuration(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)
	client.register("sess@example.com", "password123")
	client.login("sess@example.com", "password123")
	_, body := client.postJSON("/api/sites", map[string]string{"domain": "example.com"}, true)
	siteID := siteIDFromCreate(t, body)

	now := time.Now().UTC()
	visitor := make([]byte, 16)
	insert := func(session int64, name string, ago time.Duration) {
		t.Helper()
		_, err := integrationDB.Exec(`
			INSERT INTO events_raw (site_id, ts, visitor_id, session_id, name, pathname)
			VALUES ($1, $2, $3, $4, $5, '/')`, siteID, now.Add(-ago), visitor, session, name)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Session 1: two pageviews ten minutes apart -> 600s, not a bounce.
	insert(1, "pageview", 60*time.Minute)
	insert(1, "pageview", 50*time.Minute)
	// Session 2: a single pageview -> bounce, 0s.
	insert(2, "pageview", 40*time.Minute)
	// Session 3: one pageview plus a custom event -> engaged, 30s.
	insert(3, "pageview", 20*time.Minute)
	insert(3, "signup", 20*time.Minute-30*time.Second)
	// Legacy row with no session is ignored by visit metrics.
	insert(0, "pageview", 10*time.Minute)

	resp, body := client.get("/api/sites/" + itoa(siteID) + "/summary?from=" + now.Add(-24*time.Hour).Format(time.RFC3339))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var got struct {
		Pageviews     int64   `json:"pageviews"`
		Visits        int64   `json:"visits"`
		BounceRate    float64 `json:"bounce_rate"`
		VisitDuration int64   `json:"visit_duration"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Visits != 3 {
		t.Errorf("visits = %d, want 3", got.Visits)
	}
	if got.BounceRate != 33.3 {
		t.Errorf("bounce_rate = %v, want 33.3", got.BounceRate)
	}
	if got.VisitDuration != 210 { // (600 + 0 + 30) / 3
		t.Errorf("visit_duration = %d, want 210", got.VisitDuration)
	}
	if got.Pageviews != 5 {
		t.Errorf("pageviews = %d, want 5", got.Pageviews)
	}
}
