package stats

import (
	"context"
	"errors"
	"time"

	"github.com/Diptanshv/kestrel/internal/store"
)

// Dimension is a breakdown axis. Phase 5 adds country, browser, os and device
// once enrichment fills those columns.
type Dimension string

const (
	DimensionPage     Dimension = "page"
	DimensionReferrer Dimension = "referrer"
	DimensionEvent    Dimension = "event"
)

func ParseDimension(v string) (Dimension, error) {
	switch Dimension(v) {
	case DimensionPage, DimensionReferrer, DimensionEvent:
		return Dimension(v), nil
	case "":
		return "", errors.New("dimension is required")
	default:
		return "", errors.New(`dimension must be "page", "referrer" or "event"`)
	}
}

const (
	DefaultBreakdownLimit = 10
	MaxBreakdownLimit     = 100
)

// Service answers dashboard queries. In this phase every query reads
// events_raw directly; Phase 8 routes long ranges to rollups instead.
type Service struct {
	Q *store.Queries
}

func New(q *store.Queries) *Service { return &Service{Q: q} }

type Summary struct {
	Pageviews int64
	Visitors  int64
}

func (s *Service) Summary(ctx context.Context, siteID int64, r Range) (Summary, error) {
	row, err := s.Q.StatsSummary(ctx, store.StatsSummaryParams{
		SiteID: siteID,
		FromTs: r.From,
		ToTs:   r.To,
	})
	if err != nil {
		return Summary{}, err
	}
	return Summary{Pageviews: row.Pageviews, Visitors: row.Visitors}, nil
}

type Point struct {
	Bucket    time.Time
	Pageviews int64
	Visitors  int64
}

func (s *Service) Timeseries(ctx context.Context, siteID int64, r Range, interval Interval) ([]Point, error) {
	if interval == IntervalDay {
		rows, err := s.Q.StatsTimeseriesDaily(ctx, store.StatsTimeseriesDailyParams{
			SiteID: siteID,
			FromTs: r.From,
			ToTs:   r.To,
		})
		if err != nil {
			return nil, err
		}
		out := make([]Point, 0, len(rows))
		for _, row := range rows {
			out = append(out, Point{Bucket: row.Bucket, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil
	}

	rows, err := s.Q.StatsTimeseriesHourly(ctx, store.StatsTimeseriesHourlyParams{
		SiteID: siteID,
		FromTs: r.From,
		ToTs:   r.To,
	})
	if err != nil {
		return nil, err
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

func (s *Service) Breakdown(ctx context.Context, siteID int64, r Range, dim Dimension, limit int32) ([]BreakdownRow, error) {
	switch dim {
	case DimensionPage:
		rows, err := s.Q.StatsBreakdownPages(ctx, store.StatsBreakdownPagesParams{
			SiteID:   siteID,
			FromTs:   r.From,
			ToTs:     r.To,
			RowLimit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]BreakdownRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil

	case DimensionEvent:
		// Custom events only: 'pageview' is excluded by the query, so this
		// counts goal completions rather than page loads.
		rows, err := s.Q.StatsBreakdownEvents(ctx, store.StatsBreakdownEventsParams{
			SiteID:   siteID,
			FromTs:   r.From,
			ToTs:     r.To,
			RowLimit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]BreakdownRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil

	case DimensionReferrer:
		rows, err := s.Q.StatsBreakdownReferrers(ctx, store.StatsBreakdownReferrersParams{
			SiteID:   siteID,
			FromTs:   r.From,
			ToTs:     r.To,
			RowLimit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]BreakdownRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil
	}
	return nil, errors.New("unknown dimension")
}
