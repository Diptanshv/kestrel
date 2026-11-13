package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type v2Fixture struct {
	srvURL string
	client *apiClient
	siteID int64
	base   string
}

func newV2Fixture(t *testing.T) v2Fixture {
	t.Helper()
	srv, base := newTestServer(t)
	client := newAPIClient(t, srv, base)
	client.register("v2@example.com", "password123")
	client.login("v2@example.com", "password123")
	_, body := client.postJSON("/api/sites", map[string]string{"domain": "example.com"}, true)
	id := siteIDFromCreate(t, body)
	return v2Fixture{srvURL: srv.URL, client: client, siteID: id, base: "/api/sites/" + itoa(id)}
}

func (f v2Fixture) insert(t *testing.T, ago time.Duration, visitor byte, session int64, path, country string) {
	t.Helper()
	v := make([]byte, 16)
	v[0] = visitor
	_, err := integrationDB.Exec(`
		INSERT INTO events_raw (site_id, ts, visitor_id, session_id, name, pathname, country)
		VALUES ($1, $2, $3, $4, 'pageview', $5, $6)`,
		f.siteID, time.Now().Add(-ago), v, session, path, country)
	if err != nil {
		t.Fatal(err)
	}
}

// decode is curried so a (resp, body) pair can be passed straight through:
// decode[T](t)(client.get(path)).
func decode[T any](t *testing.T) func(*http.Response, []byte) T {
	return func(resp *http.Response, body []byte) T {
		t.Helper()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var v T
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		return v
	}
}

func TestRealtime(t *testing.T) {
	requireIntegration(t)
	f := newV2Fixture(t)
	f.insert(t, time.Minute, 1, 1, "/", "DE")
	f.insert(t, 2*time.Minute, 1, 1, "/a", "DE")
	f.insert(t, 3*time.Minute, 2, 2, "/", "FR")
	f.insert(t, 10*time.Minute, 3, 3, "/", "FR") // outside the 5-minute window

	got := decode[struct{ Visitors int64 }](t)(f.client.get(f.base + "/realtime"))
	if got.Visitors != 2 {
		t.Fatalf("realtime visitors = %d, want 2", got.Visitors)
	}
}

func TestFiltersNarrowAndClamp(t *testing.T) {
	requireIntegration(t)
	f := newV2Fixture(t)
	// Visitor 1 from DE: lands on / then /pricing (one visit, not a bounce).
	f.insert(t, 3*time.Hour, 1, 1, "/", "DE")
	f.insert(t, 3*time.Hour-time.Minute, 1, 1, "/pricing", "DE")
	// Visitor 2 from FR bounces on /.
	f.insert(t, 2*time.Hour, 2, 2, "/", "FR")
	// Old DE traffic, outside the 48h filter window.
	f.insert(t, 5*24*time.Hour, 3, 3, "/", "DE")

	from := url.QueryEscape(time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339))

	type summary struct {
		From       string    `json:"from"`
		Pageviews  int64     `json:"pageviews"`
		Visits     int64     `json:"visits"`
		BounceRate float64   `json:"bounce_rate"`
		Previous   *struct{} `json:"previous"`
	}
	de := decode[summary](t)(f.client.get(f.base + "/summary?country=DE&from=" + from))
	if de.Pageviews != 2 || de.Visits != 1 || de.BounceRate != 0 {
		t.Fatalf("country=DE summary = %+v, want 2 pageviews, 1 visit, 0%% bounce", de)
	}
	clampedFrom, _ := time.Parse(time.RFC3339, de.From)
	if time.Since(clampedFrom) > 49*time.Hour {
		t.Fatalf("filtered range from = %s, want clamped to the last 48h", de.From)
	}
	if de.Previous != nil {
		t.Fatal("filtered 48h range has no answerable previous period, want null")
	}

	// A page filter keeps whole sessions: visitor 1's visit touched
	// /pricing, so it counts, and is still judged by both its pageviews.
	pricing := decode[summary](t)(f.client.get(f.base + "/summary?page=/pricing"))
	if pricing.Pageviews != 1 || pricing.Visits != 1 || pricing.BounceRate != 0 {
		t.Fatalf("page=/pricing summary = %+v", pricing)
	}

	type breakdown struct {
		Rows []struct {
			Value     string `json:"value"`
			Pageviews int64  `json:"pageviews"`
		} `json:"rows"`
	}
	pages := decode[breakdown](t)(f.client.get(f.base + "/breakdown?dimension=page&country=FR"))
	if len(pages.Rows) != 1 || pages.Rows[0].Value != "/" {
		t.Fatalf("pages for FR = %+v, want just /", pages.Rows)
	}
}

func TestSummaryPreviousPeriod(t *testing.T) {
	requireIntegration(t)
	f := newV2Fixture(t)
	f.insert(t, 2*time.Hour, 1, 1, "/", "DE")
	f.insert(t, 26*time.Hour, 2, 2, "/", "DE")
	f.insert(t, 27*time.Hour, 3, 3, "/", "DE")

	from := url.QueryEscape(time.Now().Add(-24 * time.Hour).Format(time.RFC3339))
	got := decode[struct {
		Pageviews int64 `json:"pageviews"`
		Previous  *struct {
			Pageviews int64 `json:"pageviews"`
		} `json:"previous"`
	}](t)(f.client.get(f.base + "/summary?from=" + from))
	if got.Pageviews != 1 || got.Previous == nil || got.Previous.Pageviews != 2 {
		t.Fatalf("summary = %+v, previous = %+v; want 1 now, 2 before", got, got.Previous)
	}
}

func TestShareLink(t *testing.T) {
	requireIntegration(t)
	f := newV2Fixture(t)
	f.insert(t, time.Hour, 1, 1, "/", "DE")

	resp, body := f.client.postJSON(f.base+"/share", nil, true)
	site := decode[struct {
		PublicSlug *string `json:"public_slug"`
	}](t)(resp, body)
	if site.PublicSlug == nil || len(*site.PublicSlug) != 20 {
		t.Fatalf("public_slug = %v, want a 20-char slug", site.PublicSlug)
	}
	slug := *site.PublicSlug

	// Anonymous reads work, and reveal only the domain.
	anon := func(path string) (*http.Response, []byte) {
		t.Helper()
		resp, err := http.Get(f.srvURL + "/api/public/" + slug + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var buf json.RawMessage
		_ = json.NewDecoder(resp.Body).Decode(&buf)
		return resp, buf
	}
	if resp, body := anon("/"); resp.StatusCode != http.StatusOK || string(body) != `{"domain":"example.com"}` {
		t.Fatalf("public site = %d %s", resp.StatusCode, body)
	}
	sum := decode[struct{ Pageviews int64 }](t)(anon("/summary"))
	if sum.Pageviews != 1 {
		t.Fatalf("public summary pageviews = %d", sum.Pageviews)
	}

	// Enabling again is idempotent.
	resp, body = f.client.postJSON(f.base+"/share", nil, true)
	again := decode[struct {
		PublicSlug *string `json:"public_slug"`
	}](t)(resp, body)
	if again.PublicSlug == nil || *again.PublicSlug != slug {
		t.Fatal("second enable changed the slug")
	}

	// Revoking kills the link.
	if resp, body := f.client.delete(f.base + "/share"); resp.StatusCode != http.StatusOK {
		t.Fatalf("disable share = %d %s", resp.StatusCode, body)
	}
	if resp, _ := anon("/summary"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked link status = %d, want 404", resp.StatusCode)
	}
}
