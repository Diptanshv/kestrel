package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// seedEvents writes pageviews straight into events_raw. visitorByte makes each
// visitor distinct while satisfying the 16-byte check constraint.
func seedEvents(t *testing.T, siteID int64, events []struct {
	Pathname    string
	Referrer    string
	VisitorByte byte
	Ago         time.Duration
}) {
	t.Helper()
	for _, e := range events {
		visitor := make([]byte, 16)
		visitor[0] = e.VisitorByte
		var referrer any
		if e.Referrer != "" {
			referrer = e.Referrer
		}
		_, err := integrationDB.Exec(`
			INSERT INTO events_raw (site_id, ts, visitor_id, session_id, name, pathname, referrer)
			VALUES ($1, $2, $3, 0, 'pageview', $4, $5)
		`, siteID, time.Now().UTC().Add(-e.Ago), visitor, e.Pathname, referrer)
		if err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
}

func siteIDFromCreate(t *testing.T, body []byte) int64 {
	t.Helper()
	var site struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &site); err != nil {
		t.Fatalf("decode site: %v", err)
	}
	return site.ID
}

func TestStatsSummaryAndBreakdown(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)
	client.register("stats@example.com", "password123")
	client.login("stats@example.com", "password123")

	resp, body := client.postJSON("/api/sites", map[string]string{"domain": "example.com"}, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create site status = %d, body = %s", resp.StatusCode, body)
	}
	siteID := siteIDFromCreate(t, body)

	// 4 pageviews, 2 distinct visitors, /a twice.
	seedEvents(t, siteID, []struct {
		Pathname    string
		Referrer    string
		VisitorByte byte
		Ago         time.Duration
	}{
		{"/a", "news.ycombinator.com", 1, time.Hour},
		{"/a", "", 1, 2 * time.Hour},
		{"/b", "news.ycombinator.com", 2, 3 * time.Hour},
		{"/c", "", 2, 90 * time.Hour}, // outside a 48h window
	})

	t.Run("summary counts the window only", func(t *testing.T) {
		from := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
		resp, body := client.get("/api/sites/" + itoa(siteID) + "/summary?from=" + from)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var got struct {
			Pageviews int64 `json:"pageviews"`
			Visitors  int64 `json:"visitors"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Pageviews != 3 {
			t.Errorf("pageviews = %d, want 3", got.Pageviews)
		}
		if got.Visitors != 2 {
			t.Errorf("visitors = %d, want 2", got.Visitors)
		}
	})

	t.Run("top page first", func(t *testing.T) {
		resp, body := client.get("/api/sites/" + itoa(siteID) + "/breakdown?dimension=page")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var got struct {
			Rows []struct {
				Value     string `json:"value"`
				Pageviews int64  `json:"pageviews"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Rows) == 0 || got.Rows[0].Value != "/a" || got.Rows[0].Pageviews != 2 {
			t.Fatalf("rows = %+v, want /a with 2 pageviews first", got.Rows)
		}
	})

	t.Run("referrer rows exclude direct traffic", func(t *testing.T) {
		resp, body := client.get("/api/sites/" + itoa(siteID) + "/breakdown?dimension=referrer")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var got struct {
			Rows []struct {
				Value     string `json:"value"`
				Pageviews int64  `json:"pageviews"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Rows) != 1 || got.Rows[0].Value != "news.ycombinator.com" || got.Rows[0].Pageviews != 2 {
			t.Fatalf("rows = %+v, want one news.ycombinator.com row with 2", got.Rows)
		}
	})

	t.Run("timeseries has a point per bucket", func(t *testing.T) {
		from := time.Now().UTC().Add(-6 * time.Hour).Format(time.RFC3339)
		resp, body := client.get("/api/sites/" + itoa(siteID) + "/timeseries?interval=hour&from=" + from)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var got struct {
			Interval string `json:"interval"`
			Points   []struct {
				Pageviews int64 `json:"pageviews"`
			} `json:"points"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Interval != "hour" {
			t.Errorf("interval = %q, want hour", got.Interval)
		}
		// Empty hours must still appear, otherwise the chart compresses time.
		if len(got.Points) < 6 {
			t.Errorf("len(points) = %d, want at least 6", len(got.Points))
		}
	})

	t.Run("bad parameters are rejected", func(t *testing.T) {
		for _, path := range []string{
			"/summary?from=last%20tuesday",
			"/timeseries?interval=week",
			"/breakdown?dimension=country",
			"/breakdown?dimension=page&limit=0",
		} {
			resp, body := client.get("/api/sites/" + itoa(siteID) + path)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s status = %d, want 400, body = %s", path, resp.StatusCode, body)
			}
		}
	})
}

func TestStatsRequiresOwnership(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)

	owner := newAPIClient(t, srv, base)
	owner.register("owner@example.com", "password123")
	owner.login("owner@example.com", "password123")
	_, body := owner.postJSON("/api/sites", map[string]string{"domain": "example.com"}, true)
	siteID := siteIDFromCreate(t, body)

	intruder := newAPIClient(t, srv, base)
	intruder.register("intruder@example.com", "password123")
	intruder.login("intruder@example.com", "password123")

	resp, _ := intruder.get("/api/sites/" + itoa(siteID) + "/summary")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestStatsBreakdownEvents(t *testing.T) {
	requireIntegration(t)
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)
	client.register("events@example.com", "password123")
	client.login("events@example.com", "password123")

	resp, body := client.postJSON("/api/sites", map[string]string{"domain": "example.com"}, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create site status = %d, body = %s", resp.StatusCode, body)
	}
	siteID := siteIDFromCreate(t, body)

	// Two signups from one visitor, one download from another, plus a
	// pageview that must not appear in the event breakdown.
	rows := []struct {
		name    string
		visitor byte
	}{
		{"signup", 1},
		{"signup", 1},
		{"download", 2},
		{"pageview", 1},
	}
	for _, row := range rows {
		visitor := make([]byte, 16)
		visitor[0] = row.visitor
		_, err := integrationDB.Exec(`
			INSERT INTO events_raw (site_id, ts, visitor_id, session_id, name, pathname)
			VALUES ($1, now(), $2, 0, $3, '/')
		`, siteID, visitor, row.name)
		if err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	resp, body = client.get("/api/sites/" + itoa(siteID) + "/breakdown?dimension=event")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var got struct {
		Dimension string `json:"dimension"`
		Rows      []struct {
			Value     string `json:"value"`
			Pageviews int64  `json:"pageviews"`
			Visitors  int64  `json:"visitors"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Dimension != "event" {
		t.Errorf("dimension = %q, want event", got.Dimension)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %+v, want 2 (pageview must be excluded)", got.Rows)
	}
	if got.Rows[0].Value != "signup" || got.Rows[0].Pageviews != 2 || got.Rows[0].Visitors != 1 {
		t.Errorf("rows[0] = %+v, want signup with 2 events from 1 visitor", got.Rows[0])
	}
	if got.Rows[1].Value != "download" || got.Rows[1].Pageviews != 1 {
		t.Errorf("rows[1] = %+v, want download with 1 event", got.Rows[1])
	}
}
