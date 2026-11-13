package rollup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Diptanshv/kestrel/internal/testdb"
)

var pg *sql.DB

func TestMain(m *testing.M) {
	db, cleanup, err := testdb.Setup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres tests skipped: %v\n", err)
		os.Exit(m.Run())
	}
	pg = db
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func setup(t *testing.T) (*Job, int64) {
	t.Helper()
	if pg == nil {
		t.Skip("integration database not available")
	}
	testdb.Reset(t, pg)
	var uid, sid int64
	if err := pg.QueryRow(`INSERT INTO users (email, password_hash) VALUES ('r@example.com', 'x') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := pg.QueryRow(`INSERT INTO sites (user_id, domain) VALUES ($1, 'rollup.example') RETURNING id`, uid).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	return New(pg, DefaultConfig(ist), slog.New(slog.DiscardHandler)), sid
}

func insertEvent(t *testing.T, site int64, ts time.Time, visitor byte, session int64, name, path string) {
	t.Helper()
	_, err := pg.Exec(`
		INSERT INTO events_raw (site_id, ts, visitor_id, session_id, name, pathname, referrer, country)
		VALUES ($1, $2, $3, $4, $5, $6, 'news.ycombinator.com', 'DE')`,
		site, ts, vid(visitor), session, name, path)
	if err != nil {
		t.Fatal(err)
	}
}

type snapshot struct {
	Hourly, Daily []string
}

func snap(t *testing.T) snapshot {
	t.Helper()
	read := func(q string) []string {
		rows, err := pg.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}
	return snapshot{
		Hourly: read(`SELECT concat_ws('|', bucket, dimension, value, pageviews, visitors, visits, bounces, duration_sum) FROM rollup_hourly ORDER BY 1`),
		Daily:  read(`SELECT concat_ws('|', day, dimension, value, pageviews, visitors, visits, bounces, duration_sum) FROM rollup_daily ORDER BY 1`),
	}
}

func seedTraffic(t *testing.T, site int64, now time.Time) {
	t.Helper()
	// Three days ago: two visits.
	d3 := now.Add(-72 * time.Hour)
	insertEvent(t, site, d3, 1, 101, "pageview", "/")
	insertEvent(t, site, d3.Add(5*time.Minute), 1, 101, "pageview", "/pricing")
	insertEvent(t, site, d3.Add(20*time.Minute), 2, 102, "pageview", "/")
	// Recent hours, including the current open hour.
	for i := range 6 {
		ts := now.Add(-time.Duration(i) * 50 * time.Minute)
		insertEvent(t, site, ts, byte(10+i), int64(200+i), "pageview", fmt.Sprintf("/post/%d", i%2))
	}
	insertEvent(t, site, now.Add(-10*time.Minute), 10, 200, "signup", "/post/0")
}

func TestRunIsIdempotent(t *testing.T) {
	job, site := setup(t)
	now := time.Now()
	job.Now = func() time.Time { return now }
	seedTraffic(t, site, now)
	ctx := context.Background()

	first, err := job.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.HourlyRows == 0 || first.DailyRows == 0 {
		t.Fatalf("nothing rolled up: %+v", first)
	}
	a := snap(t)

	// Same instant again: the re-roll window is recomputed and must come out
	// identical.
	if _, err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if b := snap(t); !reflect.DeepEqual(a, b) {
		t.Fatalf("second run changed rollups\nfirst:  %v\nsecond: %v", a, b)
	}
}

func TestRetentionKeepsRollups(t *testing.T) {
	job, site := setup(t)
	now := time.Now()
	job.Now = func() time.Time { return now }
	seedTraffic(t, site, now)

	res, err := job.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.DeletedRaw != 3 {
		t.Fatalf("deleted_raw = %d, want 3 (the events from three days ago)", res.DeletedRaw)
	}
	var pvs int
	day := AsDate(TruncDay(now.Add(-72*time.Hour), ist))
	if err := pg.QueryRow(`SELECT pageviews FROM rollup_daily WHERE day = $1 AND dimension = '_total'`, day).Scan(&pvs); err != nil {
		t.Fatalf("daily row for %s: %v", day.Format(time.DateOnly), err)
	}
	if pvs != 3 {
		t.Fatalf("daily pageviews = %d, want 3", pvs)
	}
}

func TestAdvisoryLockSerialisesRuns(t *testing.T) {
	job, _ := setup(t)
	ctx := context.Background()

	conn, err := pg.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		t.Fatal(err)
	}

	if _, err := job.Run(ctx); !errors.Is(err, ErrBusy) {
		t.Fatalf("Run while locked: err = %v, want ErrBusy", err)
	}

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey); err != nil {
		t.Fatal(err)
	}
	if _, err := job.Run(ctx); err != nil {
		t.Fatalf("Run after unlock: %v", err)
	}
}

func TestResumesFromState(t *testing.T) {
	job, site := setup(t)
	now := time.Now()
	job.Now = func() time.Time { return now }
	insertEvent(t, site, now.Add(-30*time.Hour), 1, 1, "pageview", "/")
	if _, err := job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	// An hour later a late event lands in the previous, already-rolled hour.
	later := now.Add(time.Hour)
	job.Now = func() time.Time { return later }
	prevHour := TruncHour(now, ist).Add(-30 * time.Minute)
	insertEvent(t, site, prevHour, 2, 2, "pageview", "/late")

	res, err := job.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.HourlyFrom.Before(TruncHour(now, ist)) {
		t.Fatalf("hourly_from = %v: the re-roll window did not reach back", res.HourlyFrom)
	}
	var n int
	if err := pg.QueryRow(`SELECT count(*) FROM rollup_hourly WHERE dimension = 'page' AND value = '/late'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("late event was not picked up by the re-roll")
	}
}
