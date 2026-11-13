package stats

import (
	"context"
	"errors"
	"time"

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

// Service answers dashboard queries. In this phase every query reads
// events_raw directly; Phase 8 routes long ranges to rollups instead.
type Service struct {
	Q *store.Queries
	// Timezone the time series buckets in. Empty means UTC.
	Timezone string
}

func New(q *store.Queries, timezone string) *Service {
	if timezone == "" {
		timezone = "UTC"
	}
	return &Service{Q: q, Timezone: timezone}
}

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
			Tz:     s.Timezone,
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
		Tz:     s.Timezone,
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
			SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
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
		rows, err := s.Q.StatsBreakdownEvents(ctx, store.StatsBreakdownEventsParams{
			SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
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
			SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]BreakdownRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil

	case DimensionCountry:
		rows, err := s.Q.StatsBreakdownCountries(ctx, store.StatsBreakdownCountriesParams{
			SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]BreakdownRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil

	case DimensionBrowser:
		rows, err := s.Q.StatsBreakdownBrowsers(ctx, store.StatsBreakdownBrowsersParams{
			SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]BreakdownRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil

	case DimensionOS:
		rows, err := s.Q.StatsBreakdownOS(ctx, store.StatsBreakdownOSParams{
			SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]BreakdownRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, BreakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		return out, nil

	case DimensionDevice:
		rows, err := s.Q.StatsBreakdownDevices(ctx, store.StatsBreakdownDevicesParams{
			SiteID: siteID, FromTs: r.From, ToTs: r.To, RowLimit: limit,
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
