// Package batch moves enriched events from the request path to Postgres.
// Handlers enqueue without blocking; one goroutine drains the queue and writes
// in bulk. See docs/benchmarks.md for why.
package batch

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Event is one events_raw row, fully enriched.
type Event = store.CreateEventRawParams

// Writer persists a slice of events. Implementations must either write all of
// them or return an error; the batcher does not retry partial writes.
type Writer interface {
	Write(ctx context.Context, events []Event) error
}

var eventColumns = []string{
	"site_id", "ts", "visitor_id", "session_id", "name", "pathname",
	"referrer", "utm_source", "country", "browser", "os", "device",
}

// CopyWriter bulk-loads with the COPY protocol. The app talks to Postgres
// through database/sql, so it borrows the underlying pgx connection for the
// duration of the copy.
type CopyWriter struct {
	DB *sql.DB
}

func (w CopyWriter) Write(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	conn, err := w.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	return conn.Raw(func(driverConn any) error {
		pc, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("copy: driver conn is %T, want *stdlib.Conn", driverConn)
		}
		src := pgx.CopyFromSlice(len(events), func(i int) ([]any, error) {
			e := events[i]
			return []any{
				e.SiteID, e.Ts, e.VisitorID, e.SessionID, e.Name, e.Pathname,
				nullable(e.Referrer), nullable(e.UtmSource), nullable(e.Country),
				nullable(e.Browser), nullable(e.Os), nullable(e.Device),
			}, nil
		})
		_, err := pc.Conn().CopyFrom(ctx, pgx.Identifier{"events_raw"}, eventColumns, src)
		return err
	})
}

// InsertWriter issues one INSERT per event. It is the Phase 3 behaviour, kept
// as the benchmark baseline and for tests that want rows visible immediately.
type InsertWriter struct {
	Q *store.Queries
}

func (w InsertWriter) Write(ctx context.Context, events []Event) error {
	for _, e := range events {
		if _, err := w.Q.CreateEventRaw(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func nullable(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
}
