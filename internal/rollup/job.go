// Package rollup turns short-lived raw events into hourly and daily summary
// rows, then deletes what has aged out. Every run recomputes whole buckets and
// replaces them, so running it twice, or concurrently from the ticker and the
// cron trigger, gives the same result.
package rollup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Diptanshv/kestrel/internal/metrics"
	"github.com/Diptanshv/kestrel/internal/store"
)

// lockKey is the pg advisory lock id. Arbitrary, but must be unique within
// the database.
const lockKey int64 = 0x6b657374726f6c6c // "kestroll"

// ErrBusy means another run holds the lock. Callers treat it as success.
var ErrBusy = errors.New("rollup already running")

type Config struct {
	// Loc is the time zone buckets are cut in. It must match the stats
	// service's, or rollup rows and raw buckets will not line up.
	Loc *time.Location
	// RawRetention is how long events_raw rows are kept.
	RawRetention time.Duration
	// HourlyRetention is how long rollup_hourly rows are kept.
	HourlyRetention time.Duration
	// SaltRetentionDays matches enrich's own pruning.
	SaltRetentionDays int
	// Reroll recomputes this much of the already-rolled past on every run, to
	// pick up events flushed late and sessions that were still open.
	Reroll time.Duration
	// SessionLookback is how far before the window events are read, only to
	// recognise sessions that started earlier.
	SessionLookback time.Duration
	// TopN caps distinct values per bucket and dimension.
	TopN int
}

func DefaultConfig(loc *time.Location) Config {
	return Config{
		Loc:               loc,
		RawRetention:      48 * time.Hour,
		HourlyRetention:   30 * 24 * time.Hour,
		SaltRetentionDays: 2,
		Reroll:            2 * time.Hour,
		SessionLookback:   3 * time.Hour,
		TopN:              50,
	}
}

type Job struct {
	DB  *sql.DB
	Q   *store.Queries
	Cfg Config
	Log *slog.Logger
	Now func() time.Time
}

func New(db *sql.DB, cfg Config, log *slog.Logger) *Job {
	return &Job{DB: db, Q: store.New(db), Cfg: cfg, Log: log, Now: time.Now}
}

// Result describes one run, for logs and the /internal/rollup response.
type Result struct {
	HourlyFrom    time.Time `json:"hourly_from"`
	HourlyTo      time.Time `json:"hourly_to"`
	HourlyRows    int       `json:"hourly_rows"`
	DailyFrom     string    `json:"daily_from"`
	DailyTo       string    `json:"daily_to"`
	DailyRows     int       `json:"daily_rows"`
	DeletedRaw    int64     `json:"deleted_raw"`
	DeletedHourly int64     `json:"deleted_hourly"`
	DurationMS    int64     `json:"duration_ms"`
}

// Run performs one rollup pass under a Postgres advisory lock.
func (j *Job) Run(ctx context.Context) (Result, error) {
	start := time.Now()

	// Session-level advisory locks belong to a connection, so hold one for
	// the whole run rather than letting the pool hand out different ones.
	conn, err := j.DB.Conn(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = conn.Close() }()

	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&locked); err != nil {
		return Result{}, fmt.Errorf("advisory lock: %w", err)
	}
	if !locked {
		return Result{}, ErrBusy
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	res, err := j.run(ctx)
	res.DurationMS = time.Since(start).Milliseconds()
	metrics.RollupDuration.Observe(time.Since(start).Seconds())
	if err == nil {
		metrics.RollupLastSuccess.SetToCurrentTime()
	}
	return res, err
}

func (j *Job) run(ctx context.Context) (Result, error) {
	cfg := j.Cfg
	now := j.Now()
	var res Result

	hourFrom, dayFrom, err := j.resumePoint(ctx, now)
	if err != nil {
		return res, err
	}
	hourTo := TruncHour(now, cfg.Loc)
	dayTo := TruncDay(now, cfg.Loc)
	if hourFrom.After(hourTo) {
		hourFrom = hourTo
	}
	if dayFrom.After(dayTo) {
		dayFrom = dayTo
	}
	res.HourlyFrom, res.HourlyTo = hourFrom, hourTo
	res.DailyFrom, res.DailyTo = dayFrom.Format(time.DateOnly), dayTo.Format(time.DateOnly)

	events, err := j.Q.ListEventsForRollup(ctx, store.ListEventsForRollupParams{
		FromTs: hourFrom.Add(-cfg.SessionLookback),
		ToTs:   now,
	})
	if err != nil {
		return res, fmt.Errorf("load events: %w", err)
	}
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()
	q := j.Q.WithTx(tx)

	if res.HourlyRows, res.DailyRows, err = j.rebuild(ctx, q, sql.NullInt64{}, events, hourFrom, hourTo, dayFrom, dayTo); err != nil {
		return res, err
	}
	if err := q.SetRollupState(ctx, store.SetRollupStateParams{HourlyThrough: hourTo, DailyThrough: AsDate(dayTo)}); err != nil {
		return res, fmt.Errorf("save state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}

	// Retention runs only after the rows it would orphan are safely rolled
	// up. The cutoff never passes the rollup frontier, so a job that fell
	// behind cannot delete raw events it has not summarised yet.
	rawCutoff := now.Add(-cfg.RawRetention)
	if frontier := hourTo.Add(-cfg.Reroll - cfg.SessionLookback); frontier.Before(rawCutoff) {
		rawCutoff = frontier
	}
	if res.DeletedRaw, err = j.Q.DeleteEventsRawBefore(ctx, rawCutoff); err != nil {
		return res, fmt.Errorf("retention raw: %w", err)
	}
	if res.DeletedHourly, err = j.Q.DeleteRollupHourlyBefore(ctx, now.Add(-cfg.HourlyRetention)); err != nil {
		return res, fmt.Errorf("retention hourly: %w", err)
	}
	saltCutoff := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -cfg.SaltRetentionDays)
	if err := j.Q.DeleteSaltsBefore(ctx, saltCutoff); err != nil {
		return res, fmt.Errorf("retention salts: %w", err)
	}
	return res, nil
}

// rebuild replaces hourly rows in [hourFrom, hourTo) with ones computed from
// events, then rebuilds daily rows in [dayFrom, dayTo) from the hourly
// rows. site scopes every delete and read to one site; NULL means all.
func (j *Job) rebuild(ctx context.Context, q *store.Queries, site sql.NullInt64, events []store.ListEventsForRollupRow,
	hourFrom, hourTo, dayFrom, dayTo time.Time) (hourlyRows, dailyRows int, err error) {
	cfg := j.Cfg
	hourly := capValues(aggregate(events, func(t time.Time) time.Time { return TruncHour(t, cfg.Loc) }, hourFrom, hourTo), cfg.TopN)

	if err := q.DeleteRollupHourlyRange(ctx, store.DeleteRollupHourlyRangeParams{FromTs: hourFrom, ToTs: hourTo, SiteID: site}); err != nil {
		return 0, 0, fmt.Errorf("clear hourly: %w", err)
	}
	if len(hourly) > 0 {
		params, err := hourlyParams(hourly)
		if err != nil {
			return 0, 0, err
		}
		if err := q.UpsertRollupHourly(ctx, params); err != nil {
			return 0, 0, fmt.Errorf("upsert hourly: %w", err)
		}
	}

	// Days are rebuilt from the hourly rows just written, inside the same
	// transaction, so a re-rolled hour flows straight into its day.
	hourRows, err := q.ListRollupHourlyRange(ctx, store.ListRollupHourlyRangeParams{FromTs: dayFrom, ToTs: dayTo, SiteID: site})
	if err != nil {
		return 0, 0, fmt.Errorf("load hourly: %w", err)
	}
	daily, err := mergeHourly(hourRows, cfg.Loc)
	if err != nil {
		return 0, 0, err
	}
	daily = capValues(daily, cfg.TopN)

	if err := q.DeleteRollupDailyRange(ctx, store.DeleteRollupDailyRangeParams{FromDay: AsDate(dayFrom), ToDay: AsDate(dayTo), SiteID: site}); err != nil {
		return 0, 0, fmt.Errorf("clear daily: %w", err)
	}
	if len(daily) > 0 {
		if err := q.UpsertRollupDaily(ctx, dailyParams(daily)); err != nil {
			return 0, 0, fmt.Errorf("upsert daily: %w", err)
		}
	}
	return len(hourly), len(daily), nil
}

// Backfill rolls up one site's raw events from `from` to now, without
// touching other sites or the job's saved frontier. Anything that inserts
// backdated events (an import, the demo seeder) must call it: the scheduled
// job only re-rolls recent hours, and retention would otherwise delete the
// old rows before they were summarised.
func (j *Job) Backfill(ctx context.Context, siteID int64, from time.Time) (Result, error) {
	start := time.Now()
	conn, err := j.DB.Conn(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = conn.Close() }()
	// Blocking lock: a backfill should wait for a scheduled run, not skip.
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return Result{}, fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	cfg := j.Cfg
	now := j.Now()
	hourFrom, hourTo := TruncHour(from, cfg.Loc), TruncHour(now, cfg.Loc)
	dayFrom, dayTo := TruncDay(from, cfg.Loc), TruncDay(now, cfg.Loc)
	res := Result{
		HourlyFrom: hourFrom, HourlyTo: hourTo,
		DailyFrom: dayFrom.Format(time.DateOnly), DailyTo: dayTo.Format(time.DateOnly),
	}

	site := sql.NullInt64{Int64: siteID, Valid: true}
	events, err := j.Q.ListEventsForRollup(ctx, store.ListEventsForRollupParams{
		FromTs: hourFrom.Add(-cfg.SessionLookback), ToTs: now, SiteID: site,
	})
	if err != nil {
		return res, fmt.Errorf("load events: %w", err)
	}

	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()
	if res.HourlyRows, res.DailyRows, err = j.rebuild(ctx, j.Q.WithTx(tx), site, events, hourFrom, hourTo, dayFrom, dayTo); err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	res.DurationMS = time.Since(start).Milliseconds()
	return res, nil
}

// resumePoint returns where this run starts: the saved frontier minus the
// re-roll window, or the oldest raw event on a first run.
func (j *Job) resumePoint(ctx context.Context, now time.Time) (hourFrom, dayFrom time.Time, err error) {
	loc := j.Cfg.Loc
	state, err := j.Q.GetRollupState(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		oldest, err := j.Q.MinEventTs(ctx, now)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return TruncHour(oldest, loc), TruncDay(oldest, loc), nil
	case err != nil:
		return time.Time{}, time.Time{}, err
	}
	hourFrom = TruncHour(state.HourlyThrough.Add(-j.Cfg.Reroll), loc)
	// A day is rebuilt whenever one of its hours is, which is safe now that
	// days come from hourly sketches rather than raw events.
	dayFrom = FromDate(state.DailyThrough, loc)
	if d := TruncDay(hourFrom, loc); d.Before(dayFrom) {
		dayFrom = d
	}
	return hourFrom, dayFrom, nil
}

// RunEvery runs the job on a ticker until ctx ends. The first run happens
// immediately so a restart catches up straight away.
func (j *Job) RunEvery(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		res, err := j.Run(ctx)
		switch {
		case errors.Is(err, ErrBusy):
			j.Log.Info("rollup skipped: already running")
		case err != nil:
			if ctx.Err() == nil {
				j.Log.Error("rollup failed", "err", err)
			}
		default:
			j.Log.Info("rollup done", "hourly_rows", res.HourlyRows, "daily_rows", res.DailyRows,
				"deleted_raw", res.DeletedRaw, "duration_ms", res.DurationMS)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func hourlyParams(rows map[key]*agg) (store.UpsertRollupHourlyParams, error) {
	var p store.UpsertRollupHourlyParams
	for k, a := range rows {
		sk, err := a.sketch().MarshalBinary()
		if err != nil {
			return p, err
		}
		p.Sketches = append(p.Sketches, sk)
		p.SiteIds = append(p.SiteIds, k.site)
		p.Buckets = append(p.Buckets, k.bucket.UTC())
		p.Dimensions = append(p.Dimensions, k.dimension)
		p.Vals = append(p.Vals, k.value)
		p.Pageviews = append(p.Pageviews, int32(a.pageviews))
		p.Visitors = append(p.Visitors, int32(a.uniques()))
		p.Visits = append(p.Visits, int32(a.visits))
		p.Bounces = append(p.Bounces, int32(a.bounces))
		p.DurationSums = append(p.DurationSums, a.duration)
	}
	return p, nil
}

func dailyParams(rows map[key]*agg) store.UpsertRollupDailyParams {
	var p store.UpsertRollupDailyParams
	for k, a := range rows {
		p.SiteIds = append(p.SiteIds, k.site)
		p.Days = append(p.Days, AsDate(k.bucket))
		p.Dimensions = append(p.Dimensions, k.dimension)
		p.Vals = append(p.Vals, k.value)
		p.Pageviews = append(p.Pageviews, int32(a.pageviews))
		p.Visitors = append(p.Visitors, int32(a.uniques()))
		p.Visits = append(p.Visits, int32(a.visits))
		p.Bounces = append(p.Bounces, int32(a.bounces))
		p.DurationSums = append(p.DurationSums, a.duration)
	}
	return p
}
