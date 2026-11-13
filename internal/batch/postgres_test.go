package batch

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Diptanshv/kestrel/internal/store"
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

func requirePG(tb testing.TB) {
	tb.Helper()
	if pg == nil {
		tb.Skip("integration database not available")
	}
}

// seedSite resets the database and returns a fresh site id for FK purposes.
func seedSite(tb testing.TB) int64 {
	tb.Helper()
	if _, err := pg.Exec("TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		tb.Fatal(err)
	}
	var uid, sid int64
	if err := pg.QueryRow(`INSERT INTO users (email, password_hash) VALUES ('b@example.com', 'x') RETURNING id`).Scan(&uid); err != nil {
		tb.Fatal(err)
	}
	if err := pg.QueryRow(`INSERT INTO sites (user_id, domain) VALUES ($1, 'bench.example') RETURNING id`, uid).Scan(&sid); err != nil {
		tb.Fatal(err)
	}
	return sid
}

func makeEvents(siteID int64, n int) []Event {
	out := make([]Event, n)
	now := time.Now().UTC()
	for i := range out {
		vid := make([]byte, 16)
		_, _ = rand.Read(vid)
		out[i] = Event{
			SiteID:    siteID,
			Ts:        now,
			VisitorID: vid,
			Name:      "pageview",
			Pathname:  fmt.Sprintf("/p/%d", i%50),
			Referrer:  sql.NullString{String: "news.ycombinator.com", Valid: i%3 == 0},
			Country:   sql.NullString{String: "DE", Valid: true},
			Browser:   sql.NullString{String: "Firefox", Valid: true},
			Os:        sql.NullString{String: "Linux", Valid: true},
			Device:    sql.NullString{String: "desktop", Valid: true},
		}
	}
	return out
}

func countRows(tb testing.TB) int {
	tb.Helper()
	var n int
	if err := pg.QueryRow("SELECT count(*) FROM events_raw").Scan(&n); err != nil {
		tb.Fatal(err)
	}
	return n
}

func TestCopyWriterRoundTrip(t *testing.T) {
	requirePG(t)
	sid := seedSite(t)
	events := makeEvents(sid, 3)
	events[1].Referrer = sql.NullString{}
	events[1].Device = sql.NullString{}

	if err := (CopyWriter{DB: pg}).Write(context.Background(), events); err != nil {
		t.Fatal(err)
	}

	rows, err := store.New(pg).ListEventsRawBySiteID(context.Background(), store.ListEventsRawBySiteIDParams{
		SiteID: sid, Ts: time.Now().Add(-time.Hour), Ts_2: time.Now().Add(time.Hour), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	nullRefs := 0
	for _, r := range rows {
		if !r.Referrer.Valid {
			nullRefs++
		}
		if r.Country.String != "DE" || len(r.VisitorID) != 16 {
			t.Fatalf("unexpected row %+v", r)
		}
	}
	if nullRefs != 2 { // events[1] cleared, events[2] invalid by i%3
		t.Fatalf("NULL referrers = %d, want 2", nullRefs)
	}
}

// TestGracefulStopLosesNothing is the Phase 6 "drain on shutdown" guarantee
// against the real database: everything accepted before Close is persisted.
func TestGracefulStopLosesNothing(t *testing.T) {
	requirePG(t)
	sid := seedSite(t)

	b := New(CopyWriter{DB: pg}, discard(), Config{BatchSize: 500, FlushInterval: time.Hour})
	b.Start()
	const n = 2345
	for _, e := range makeEvents(sid, n) {
		if !b.Enqueue(e) {
			t.Fatal("dropped before close")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := b.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t); got != n {
		t.Fatalf("rows = %d, want %d", got, n)
	}
}

// BenchmarkWrite compares per-row INSERT (the Phase 3 path) with COPY for the
// same 500-event batch. Results are recorded in docs/benchmarks.md.
func BenchmarkWrite(b *testing.B) {
	requirePG(b)
	sid := seedSite(b)
	events := makeEvents(sid, 500)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		w    Writer
	}{
		{"insert_per_row", InsertWriter{Q: store.New(pg)}},
		{"copy_from", CopyWriter{DB: pg}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if err := tc.w.Write(ctx, events); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N*len(events))/b.Elapsed().Seconds(), "events/s")
			b.StopTimer()
			if _, err := pg.Exec("TRUNCATE events_raw"); err != nil {
				b.Fatal(err)
			}
		})
	}
}
