package stats

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/Diptanshv/kestrel/internal/rollup"
	"github.com/Diptanshv/kestrel/internal/store"
)

// Dimension is a breakdown axis.
type Dimension string

const (
	DimensionPage     Dimension = "page"
	DimensionReferrer Dimension = "referrer"
	DimensionEvent    Dimension = "event"
	DimensionCountry  Dimension = "country"
	DimensionBrowser  Dimension = "browser"
	DimensionOS       Dimension = "os"
	DimensionDevice   Dimension = "device"
)

var dimensions = map[Dimension]bool{
	DimensionPage: true, DimensionReferrer: true, DimensionEvent: true,
	DimensionCountry: true, DimensionBrowser: true, DimensionOS: true, DimensionDevice: true,
}

func ParseDimension(v string) (Dimension, error) {
	if v == "" {
		return "", errors.New("dimension is required")
	}
	if !dimensions[Dimension(v)] {
		return "", errors.New(`dimension must be one of: page, referrer, event, country, browser, os, device`)
	}
	return Dimension(v), nil
}

const (
	DefaultBreakdownLimit = 10
	MaxBreakdownLimit     = 100
)

// RawWindow is how far back events_raw is complete. Ranges that start inside
// it are answered from raw events alone; older ranges need rollups.
const RawWindow = 48 * time.Hour

// Service answers dashboard queries. Recent ranges read events_raw. Ranges
// reaching past RawWindow read rollup_daily (or rollup_hourly) for buckets
// the rollup job has closed and events_raw for the rest.
type Service struct {
	Q *store.Queries
	// Timezone the time series buckets in. Empty means UTC. It must match
	// the rollup job's, which cmd/server guarantees by passing the same one.
	Timezone string
	Loc      *time.Location
	Now      func() time.Time
}

func New(q *store.Queries, timezone string) *Service {
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc, timezone = time.UTC, "UTC"
	}
	return &Service{Q: q, Timezone: timezone, Loc: loc, Now: time.Now}
}

// plan splits a range into a rolled-up prefix and a raw suffix.
type plan struct {
	// Rolled-up part as DATE values (UTC midnights carrying local dates),
	// or hourly instants. Empty when from == to.
	rollFrom, rollTo time.Time
	raw              Range
	rawEmpty         bool
}

func (p plan) hasRollup() bool { return p.rollTo.After(p.rollFrom) }

// planDaily decides how to answer r at day granularity.
func (s *Service) planDaily(ctx context.Context, r Range) (plan, error) {
	if !r.From.Before(s.Now().Add(-RawWindow)) {
		return plan{raw: r}, nil
	}
	state, err := s.Q.GetRollupState(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return plan{raw: r}, nil // nothing rolled up yet; raw is all there is
	}
	if err != nil {
		return plan{}, err
	}
	frontier := rollup.FromDate(state.DailyThrough, s.Loc) // first day not rolled up
	start := rollup.TruncDay(r.From, s.Loc)
	end := frontier
	if r.To.Before(end) {
		end = rollup.TruncDay(r.To, s.Loc)
		if !end.Equal(r.To) {
			end = end.AddDate(0, 0, 1)
		}
	}
	p := plan{rollFrom: rollup.AsDate(start), rollTo: rollup.AsDate(start)}
	if end.After(start) {
		p.rollTo = rollup.AsDate(end)
	}
	p.raw = Range{From: maxTime(r.From, frontier), To: r.To}
	p.rawEmpty = !p.raw.To.After(p.raw.From)
	return p, nil
}

// planHourly is planDaily for hourly series, using rollup_hourly.
func (s *Service) planHourly(ctx context.Context, r Range) (plan, error) {
	if !r.From.Before(s.Now().Add(-RawWindow)) {
		return plan{raw: r}, nil
	}
	state, err := s.Q.GetRollupState(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return plan{raw: r}, nil
	}
	if err != nil {
		return plan{}, err
	}
	frontier := state.HourlyThrough
	start := rollup.TruncHour(r.From, s.Loc)
	p := plan{rollFrom: start, rollTo: start}
	if end := minTime(frontier, r.To); end.After(start) {
		p.rollTo = end
	}
	p.raw = Range{From: maxTime(r.From, frontier), To: r.To}
	p.rawEmpty = !p.raw.To.After(p.raw.From)
	return p, nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

type Summary struct {
	Pageviews   int64
	Visitors    int64
	Visits      int64
	Bounces     int64
	DurationSum int64 // seconds, summed over visits
}

// BounceRate is the share of visits that bounced, 0..1.
func (s Summary) BounceRate() float64 {
	if s.Visits == 0 {
		return 0
	}
	return float64(s.Bounces) / float64(s.Visits)
}

// AvgDuration is the mean visit length in seconds.
func (s Summary) AvgDuration() float64 {
	if s.Visits == 0 {
		return 0
	}
	return float64(s.DurationSum) / float64(s.Visits)
}

// Summary totals a range. Over rollups, visitors is the sum of daily
// uniques: visitor IDs rotate daily, so that is the only honest number.
func (s *Service) Summary(ctx context.Context, siteID int64, r Range, f Filters) (Summary, error) {
	if f.Active() {
		return s.rawSummary(ctx, siteID, r, f)
	}
	p, err := s.planDaily(ctx, r)
	if err != nil {
		return Summary{}, err
	}
	var out Summary
	if p.hasRollup() {
		row, err := s.Q.RollupDailyTotals(ctx, store.RollupDailyTotalsParams{
			SiteID: siteID, FromDay: p.rollFrom, ToDay: p.rollTo,
		})
		if err != nil {
			return Summary{}, err
		}
		out = Summary{
			Pageviews: row.Pageviews, Visitors: row.Visitors, Visits: row.Visits,
			Bounces: row.Bounces, DurationSum: row.DurationSum,
		}
	}
	if !p.rawEmpty {
		raw, err := s.rawSummary(ctx, siteID, p.raw, f)
		if err != nil {
			return Summary{}, err
		}
		out.Pageviews += raw.Pageviews
		out.Visitors += raw.Visitors
		out.Visits += raw.Visits
		out.Bounces += raw.Bounces
		out.DurationSum += raw.DurationSum
	}
	return out, nil
}

func (s *Service) rawSummary(ctx context.Context, siteID int64, r Range, f Filters) (Summary, error) {
	row, err := s.Q.StatsSummary(ctx, store.StatsSummaryParams{
		SiteID: siteID, FromTs: r.From, ToTs: r.To,
		FPage: f.Page, FReferrer: f.Referrer, FCountry: f.Country,
		FBrowser: f.Browser, FOs: f.OS, FDevice: f.Device,
	})
	if err != nil {
		return Summary{}, err
	}
	sess, err := s.Q.StatsSessions(ctx, store.StatsSessionsParams{
		SiteID: siteID, FromTs: r.From, ToTs: r.To,
		FPage: f.Page, FReferrer: f.Referrer, FCountry: f.Country,
		FBrowser: f.Browser, FOs: f.OS, FDevice: f.Device,
	})
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		Pageviews:   row.Pageviews,
		Visitors:    row.Visitors,
		Visits:      sess.Visits,
		Bounces:     sess.Bounces,
		DurationSum: sess.DurationSum,
	}, nil
}

type Point struct {
	Bucket    time.Time
	Pageviews int64
	Visitors  int64
}

func (s *Service) Timeseries(ctx context.Context, siteID int64, r Range, interval Interval, f Filters) ([]Point, error) {
	if f.Active() {
		return s.rawSeries(ctx, siteID, r, interval, f)
	}
	if interval == IntervalDay {
		return s.dailySeries(ctx, siteID, r)
	}
	return s.hourlySeries(ctx, siteID, r)
}

func (s *Service) dailySeries(ctx context.Context, siteID int64, r Range) ([]Point, error) {
	p, err := s.planDaily(ctx, r)
	if err != nil {
		return nil, err
	}
	var out []Point
	if p.hasRollup() {
		rows, err := s.Q.RollupDailySeries(ctx, store.RollupDailySeriesParams{
			SiteID: siteID, FromDay: p.rollFrom, ToDay: p.rollTo,
		})
		if err != nil {
			return nil, err
		}
		byDay := make(map[time.Time]store.RollupDailySeriesRow, len(rows))
		for _, row := range rows {
			byDay[row.Day] = row
		}
		// Days with no traffic have no rollup row; fill them like the raw
		// query's generate_series does.
		for d := p.rollFrom; d.Before(p.rollTo); d = d.AddDate(0, 0, 1) {
			row := byDay[d]
			out = append(out, Point{Bucket: rollup.FromDate(d, s.Loc), Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
	}
	if !p.rawEmpty {
		raw, err := s.rawSeries(ctx, siteID, p.raw, IntervalDay, Filters{})
		if err != nil {
			return nil, err
		}
		out = append(out, raw...)
	}
	return out, nil
}

func (s *Service) hourlySeries(ctx context.Context, siteID int64, r Range) ([]Point, error) {
	p, err := s.planHourly(ctx, r)
	if err != nil {
		return nil, err
	}
	var out []Point
	if p.hasRollup() {
		rows, err := s.Q.RollupHourlySeries(ctx, store.RollupHourlySeriesParams{
			SiteID: siteID, FromTs: p.rollFrom, ToTs: p.rollTo,
		})
		if err != nil {
			return nil, err
		}
		byHour := make(map[int64]store.RollupHourlySeriesRow, len(rows))
		for _, row := range rows {
			byHour[row.Bucket.Unix()] = row
		}
		for h := p.rollFrom; h.Before(p.rollTo); h = h.Add(time.Hour) {
			row := byHour[h.Unix()]
			out = append(out, Point{Bucket: h, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
	}
	if !p.rawEmpty {
		raw, err := s.rawSeries(ctx, siteID, p.raw, IntervalHour, Filters{})
		if err != nil {
			return nil, err
		}
		out = append(out, raw...)
	}
	return out, nil
}

func (s *Service) rawSeries(ctx context.Context, siteID int64, r Range, interval Interval, f Filters) ([]Point, error) {
	// Both queries take identical parameters and return identical rows, so
	// one params value converts to the other.
	p := store.StatsTimeseriesHourlyParams{
		FromTs: r.From, ToTs: r.To, Tz: s.Timezone, SiteID: siteID,
		FPage: f.Page, FReferrer: f.Referrer, FCountry: f.Country,
		FBrowser: f.Browser, FOs: f.OS, FDevice: f.Device,
	}
	var rows []store.StatsTimeseriesHourlyRow
	if interval == IntervalDay {
		daily, err := s.Q.StatsTimeseriesDaily(ctx, store.StatsTimeseriesDailyParams(p))
		if err != nil {
			return nil, err
		}
		for _, d := range daily {
			rows = append(rows, store.StatsTimeseriesHourlyRow(d))
		}
	} else {
		var err error
		if rows, err = s.Q.StatsTimeseriesHourly(ctx, p); err != nil {
			return nil, err
		}
	}
	out := make([]Point, 0, len(rows))
	for _, row := range rows {
		out = append(out, Point{Bucket: row.Bucket, Pageviews: row.Pageviews, Visitors: row.Visitors})
	}
	return out, nil
}

type BreakdownRow struct {
	Value     string
	Pageviews int64
	Visitors  int64
}

// Breakdown lists the top values of a dimension. Over rollups, values that
// fell outside a day's top 50 appear as a single "(other)" row.
func (s *Service) Breakdown(ctx context.Context, siteID int64, r Range, dim Dimension, limit int32, f Filters) ([]BreakdownRow, error) {
	if f.Active() {
		return s.rawBreakdown(ctx, siteID, r, dim, limit, f)
	}
	p, err := s.planDaily(ctx, r)
	if err != nil {
		return nil, err
	}
	if !p.hasRollup() {
		return s.rawBreakdown(ctx, siteID, p.raw, dim, limit, f)
	}

	merged := make(map[string]*BreakdownRow)
	add := func(v string, pv, vis int64) {
		row, ok := merged[v]
		if !ok {
			row = &BreakdownRow{Value: v}
			merged[v] = row
		}
		row.Pageviews += pv
		row.Visitors += vis
	}
	rows, err := s.Q.RollupDailyBreakdown(ctx, store.RollupDailyBreakdownParams{
		SiteID: siteID, Dimension: string(dim), FromDay: p.rollFrom, ToDay: p.rollTo,
		RowLimit: MaxBreakdownLimit,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		add(row.Value, row.Pageviews, row.Visitors)
	}
	if !p.rawEmpty {
		raw, err := s.rawBreakdown(ctx, siteID, p.raw, dim, MaxBreakdownLimit, f)
		if err != nil {
			return nil, err
		}
		for _, row := range raw {
			add(row.Value, row.Pageviews, row.Visitors)
		}
	}

	out := make([]BreakdownRow, 0, len(merged))
	for _, row := range merged {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pageviews != out[j].Pageviews {
			return out[i].Pageviews > out[j].Pageviews
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > int(limit) {
		out = out[:limit]
	}
	return out, nil
}

func (s *Service) rawBreakdown(ctx context.Context, siteID int64, r Range, dim Dimension, limit int32, f Filters) ([]BreakdownRow, error) {
	// Every breakdown query has the same parameter and row shape; build one
	// params value and convert it per query.
	p := store.StatsBreakdownPagesParams{
		SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
		FPage: f.Page, FReferrer: f.Referrer, FCountry: f.Country,
		FBrowser: f.Browser, FOs: f.OS, FDevice: f.Device,
	}
	var (
		rows []store.StatsBreakdownPagesRow
		err  error
	)
	switch dim {
	case DimensionPage:
		rows, err = s.Q.StatsBreakdownPages(ctx, p)
	case DimensionEvent:
		rows, err = convertRows(s.Q.StatsBreakdownEvents(ctx, store.StatsBreakdownEventsParams(p)))
	case DimensionReferrer:
		rows, err = convertRows(s.Q.StatsBreakdownReferrers(ctx, store.StatsBreakdownReferrersParams(p)))
	case DimensionCountry:
		rows, err = convertRows(s.Q.StatsBreakdownCountries(ctx, store.StatsBreakdownCountriesParams(p)))
	case DimensionBrowser:
		rows, err = convertRows(s.Q.StatsBreakdownBrowsers(ctx, store.StatsBreakdownBrowsersParams(p)))
	case DimensionOS:
		rows, err = convertRows(s.Q.StatsBreakdownOS(ctx, store.StatsBreakdownOSParams(p)))
	case DimensionDevice:
		rows, err = convertRows(s.Q.StatsBreakdownDevices(ctx, store.StatsBreakdownDevicesParams(p)))
	default:
		return nil, errors.New("unknown dimension")
	}
	if err != nil {
		return nil, err
	}
	out := make([]BreakdownRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
	}
	return out, nil
}

// breakdownRow is satisfied by every generated breakdown row type, which all
// share the pages row's layout.
type breakdownRow interface {
	store.StatsBreakdownEventsRow | store.StatsBreakdownReferrersRow | store.StatsBreakdownCountriesRow |
		store.StatsBreakdownBrowsersRow | store.StatsBreakdownOSRow | store.StatsBreakdownDevicesRow
}

func convertRows[R breakdownRow](rows []R, err error) ([]store.StatsBreakdownPagesRow, error) {
	if err != nil {
		return nil, err
	}
	out := make([]store.StatsBreakdownPagesRow, len(rows))
	for i, r := range rows {
		out[i] = store.StatsBreakdownPagesRow(r)
	}
	return out, nil
}

// Filters narrow raw queries to events matching every set field. They only
// apply to the raw window: rollups store each dimension on its own, so they
// cannot answer "pages, for visitors from Germany".
type Filters struct {
	Page, Referrer, Country, Browser, OS, Device sql.NullString
}

// FilterKeys are the query parameters ParseFilters reads, in display order.
var FilterKeys = []string{"page", "referrer", "country", "browser", "os", "device"}

// ParseFilters reads filters from a lookup such as url.Values.Get.
func ParseFilters(get func(string) string) Filters {
	ns := func(k string) sql.NullString {
		v := get(k)
		return sql.NullString{String: v, Valid: v != ""}
	}
	return Filters{
		Page: ns("page"), Referrer: ns("referrer"), Country: ns("country"),
		Browser: ns("browser"), OS: ns("os"), Device: ns("device"),
	}
}

// Active reports whether any filter is set.
func (f Filters) Active() bool {
	return f.Page.Valid || f.Referrer.Valid || f.Country.Valid || f.Browser.Valid || f.OS.Valid || f.Device.Valid
}

// Realtime counts distinct visitors seen in the last five minutes.
func (s *Service) Realtime(ctx context.Context, siteID int64) (int64, error) {
	return s.Q.StatsRealtime(ctx, siteID)
}
