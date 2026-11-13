package rollup

import (
	"context"
	"encoding/binary"
	"math"
	mrand "math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/Diptanshv/kestrel/internal/batch"
)

// seedDay writes events for `visitors` distinct visitors, two pageviews each,
// spread over yesterday (local), and returns the exact COUNT(DISTINCT).
func seedDay(t *testing.T, site int64, now time.Time, visitors int) (time.Time, int) {
	t.Helper()
	day := TruncDay(now, ist).AddDate(0, 0, -1)
	r := mrand.New(mrand.NewPCG(uint64(visitors), 7))
	events := make([]batch.Event, 0, 2*visitors)
	for i := range visitors {
		v := make([]byte, 16)
		binary.BigEndian.PutUint64(v, r.Uint64())
		binary.BigEndian.PutUint64(v[8:], r.Uint64())
		for range 2 {
			ts := day.Add(time.Duration(r.Int64N(int64(24 * time.Hour))))
			events = append(events, batch.Event{SiteID: site, Ts: ts, VisitorID: v, SessionID: int64(i + 1), Name: "pageview", Pathname: "/"})
		}
	}
	if err := (batch.CopyWriter{DB: pg}).Write(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	var exact int
	err := pg.QueryRow(`SELECT count(DISTINCT visitor_id) FROM events_raw WHERE ts >= $1 AND ts < $2`, day, day.AddDate(0, 0, 1)).Scan(&exact)
	if err != nil {
		t.Fatal(err)
	}
	return day, exact
}

func dailyVisitors(t *testing.T, day time.Time) int {
	t.Helper()
	var n int
	if err := pg.QueryRow(`SELECT visitors FROM rollup_daily WHERE day = $1 AND dimension = '_total'`, AsDate(day)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestDailyUniquesMatchExact checks the Phase 9 "done when": a day's
// visitors, merged from 24 hourly sketches, are within a few percent of
// COUNT(DISTINCT) over the raw events.
func TestDailyUniquesMatchExact(t *testing.T) {
	job, site := setup(t)
	now := time.Now()
	job.Now = func() time.Time { return now }

	day, exact := seedDay(t, site, now, 3000)
	if _, err := job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := dailyVisitors(t, day)
	if e := math.Abs(float64(got-exact)) / float64(exact); e > 0.05 {
		t.Fatalf("daily visitors = %d, exact = %d, error %.2f%%", got, exact, 100*e)
	}
}

// TestUniquesAccuracyReport runs the same comparison at several sizes:
//
//	KESTREL_BENCH=1 go test ./internal/rollup -run TestUniquesAccuracyReport -v
func TestUniquesAccuracyReport(t *testing.T) {
	if os.Getenv("KESTREL_BENCH") == "" {
		t.Skip("set KESTREL_BENCH=1 to run")
	}
	for _, n := range []int{100, 1_000, 10_000, 50_000, 100_000} {
		job, site := setup(t)
		now := time.Now()
		job.Now = func() time.Time { return now }
		day, exact := seedDay(t, site, now, n)
		res, err := job.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got := dailyVisitors(t, day)
		var hourlyBytes, hourlyRows int
		_ = pg.QueryRow(`SELECT coalesce(sum(octet_length(visitors_hll)),0), count(*) FROM rollup_hourly WHERE dimension = '_total'`).Scan(&hourlyBytes, &hourlyRows)
		t.Logf("exact %-7d hll %-7d error %+.2f%%  rollup %dms  avg hourly sketch %d B",
			exact, got, 100*float64(got-exact)/float64(exact), res.DurationMS, hourlyBytes/max(hourlyRows, 1))
	}
}
