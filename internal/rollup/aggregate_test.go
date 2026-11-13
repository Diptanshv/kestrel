package rollup

import (
	"fmt"
	"testing"
	"time"

	"github.com/Diptanshv/kestrel/internal/store"
)

var ist = time.FixedZone("IST", 5*3600+1800)

func vid(b byte) []byte { v := make([]byte, 16); v[0] = b; return v }

func pv(ts time.Time, visitor byte, session int64, path string) store.ListEventsForRollupRow {
	return store.ListEventsForRollupRow{
		SiteID: 1, Ts: ts, VisitorID: vid(visitor), SessionID: session,
		Name: "pageview", Pathname: path, Country: "IN", Browser: "Firefox",
	}
}

func TestTruncHourFollowsZone(t *testing.T) {
	// 10:45 UTC is 16:15 IST; the IST hour starts at 16:00 IST = 10:30 UTC.
	got := TruncHour(time.Date(2026, 11, 13, 10, 45, 0, 0, time.UTC), ist)
	want := time.Date(2026, 11, 13, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("TruncHour = %v, want %v", got.UTC(), want)
	}
	// 20:00 UTC on the 13th is 01:30 IST on the 14th.
	day := TruncDay(time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC), ist)
	if d := AsDate(day); d.Format(time.DateOnly) != "2026-11-14" {
		t.Fatalf("local date = %s, want 2026-11-14", d.Format(time.DateOnly))
	}
}

func TestAggregate(t *testing.T) {
	h := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	hour := func(t time.Time) time.Time { return TruncHour(t, time.UTC) }

	events := []store.ListEventsForRollupRow{
		// Session 9 started before the window: its pageview inside the window
		// counts, but the visit does not (it belongs to the earlier hour).
		pv(h.Add(-20*time.Minute), 9, 9, "/old"),
		pv(h.Add(5*time.Minute), 9, 9, "/a"),

		// Session 1: two pageviews, 10 minutes, entry /a.
		pv(h.Add(1*time.Minute), 1, 1, "/a"),
		pv(h.Add(11*time.Minute), 1, 1, "/b"),
		// Session 2: bounce on /a.
		pv(h.Add(30*time.Minute), 2, 2, "/a"),
		// Session 3: pageview + custom event, not a bounce, in the next hour.
		pv(h.Add(70*time.Minute), 3, 3, "/c"),
		{SiteID: 1, Ts: h.Add(71 * time.Minute), VisitorID: vid(3), SessionID: 3, Name: "signup", Pathname: "/c"},
	}

	rows := aggregate(events, hour, h, h.Add(2*time.Hour))
	at := func(bucket time.Time, dim, val string) *agg {
		a := rows[key{1, bucket, dim, val}]
		if a == nil {
			t.Fatalf("missing row %s %s=%q", bucket.Format("15:04"), dim, val)
		}
		return a
	}

	total := at(h, DimTotal, "")
	if total.pageviews != 4 || total.uniques() != 3 {
		t.Errorf("10:00 total pageviews=%d visitors=%d, want 4, 3", total.pageviews, total.uniques())
	}
	if total.visits != 2 || total.bounces != 1 || total.duration != 600 {
		t.Errorf("10:00 total visits=%d bounces=%d duration=%d, want 2, 1, 600", total.visits, total.bounces, total.duration)
	}

	pageA := at(h, "page", "/a")
	if pageA.pageviews != 3 || pageA.visits != 2 || pageA.bounces != 1 {
		t.Errorf("/a pageviews=%d visits=%d bounces=%d, want 3, 2, 1", pageA.pageviews, pageA.visits, pageA.bounces)
	}
	// /b was never an entry page.
	if b := at(h, "page", "/b"); b.visits != 0 || b.pageviews != 1 {
		t.Errorf("/b visits=%d pageviews=%d, want 0, 1", b.visits, b.pageviews)
	}
	if _, ok := rows[key{1, h, "page", "/old"}]; ok {
		t.Error("event before the window was counted")
	}

	next := at(h.Add(time.Hour), DimTotal, "")
	if next.visits != 1 || next.bounces != 0 || next.duration != 60 {
		t.Errorf("11:00 visits=%d bounces=%d duration=%d, want 1, 0, 60", next.visits, next.bounces, next.duration)
	}
	if ev := at(h.Add(time.Hour), "event", "signup"); ev.pageviews != 1 {
		t.Errorf("signup count = %d, want 1", ev.pageviews)
	}
	// Custom events do not count as pageviews in the total.
	if next.pageviews != 1 {
		t.Errorf("11:00 pageviews = %d, want 1", next.pageviews)
	}
}

func TestCapValuesFoldsTail(t *testing.T) {
	h := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	var events []store.ListEventsForRollupRow
	// /p0 gets 10 views, /p1 9, ... /p9 1.
	for i := range 10 {
		for j := 0; j < 10-i; j++ {
			events = append(events, pv(h.Add(time.Minute), byte(j), 0, fmt.Sprintf("/p%d", i)))
		}
	}
	rows := capValues(aggregate(events, func(t time.Time) time.Time { return h }, h, h.Add(time.Hour)), 3)

	pages := 0
	for k := range rows {
		if k.dimension == "page" {
			pages++
		}
	}
	if pages != 4 {
		t.Fatalf("page rows = %d, want 3 kept + (other)", pages)
	}
	other := rows[key{1, h, "page", Other}]
	if other == nil || other.pageviews != 7+6+5+4+3+2+1 {
		t.Fatalf("(other) = %+v, want 28 pageviews", other)
	}
	if other.uniques() != 7 { // visitors 0..6 appear on /p3
		t.Fatalf("(other) visitors = %d, want 7 (deduplicated union)", other.uniques())
	}
	if rows[key{1, h, DimTotal, ""}].pageviews != 55 {
		t.Fatal("total must not be capped")
	}
}
