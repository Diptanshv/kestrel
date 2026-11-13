package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Diptanshv/kestrel/internal/stats"
	"github.com/Diptanshv/kestrel/internal/store"
)

type summaryMetrics struct {
	Pageviews int64 `json:"pageviews"`
	Visitors  int64 `json:"visitors"`
	Visits    int64 `json:"visits"`
	// BounceRate is a percentage, 0..100, rounded to one decimal.
	BounceRate float64 `json:"bounce_rate"`
	// VisitDuration is the mean visit length in whole seconds.
	VisitDuration int64 `json:"visit_duration"`
}

type summaryResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
	summaryMetrics
	// Previous covers the same-length window just before From, for change
	// badges. Null when it cannot be answered (filtered ranges whose
	// previous window falls outside the raw retention).
	Previous *summaryMetrics `json:"previous"`
}

func toMetrics(s stats.Summary) summaryMetrics {
	return summaryMetrics{
		Pageviews:     s.Pageviews,
		Visitors:      s.Visitors,
		Visits:        s.Visits,
		BounceRate:    math.Round(s.BounceRate()*1000) / 10,
		VisitDuration: int64(math.Round(s.AvgDuration())),
	}
}

type timeseriesPoint struct {
	Bucket    string `json:"bucket"`
	Pageviews int64  `json:"pageviews"`
	Visitors  int64  `json:"visitors"`
}

type timeseriesResponse struct {
	Interval string            `json:"interval"`
	Points   []timeseriesPoint `json:"points"`
}

type breakdownRow struct {
	Value     string `json:"value"`
	Pageviews int64  `json:"pageviews"`
	Visitors  int64  `json:"visitors"`
}

type breakdownResponse struct {
	Dimension string         `json:"dimension"`
	Rows      []breakdownRow `json:"rows"`
}

// siteLoader resolves the site a stats request is about and writes the error
// response itself when it cannot. Owners load by id behind a session; public
// dashboards load by slug with no auth. Everything after that is shared.
type siteLoader func(w http.ResponseWriter, r *http.Request) (store.Site, bool)

// statsQuery is the parsed range and filters of a stats request. Filters
// only work on raw events, so a filtered range is clamped to the raw window;
// the response's from/to report what was actually used.
func statsQuery(w http.ResponseWriter, r *http.Request) (stats.Range, stats.Filters, bool) {
	q := r.URL.Query()
	now := time.Now()
	rng, err := stats.ParseRange(q.Get("from"), q.Get("to"), now)
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return stats.Range{}, stats.Filters{}, false
	}
	f := stats.ParseFilters(q.Get)
	if f.Active() {
		if floor := now.UTC().Add(-stats.RawWindow); rng.From.Before(floor) {
			rng.From = floor
		}
		if !rng.To.After(rng.From) {
			_ = writeError(w, http.StatusBadRequest, "invalid_request", "filters only cover the last 48 hours")
			return stats.Range{}, stats.Filters{}, false
		}
	}
	return rng, f, true
}

func (api *API) summaryHandler(load siteLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := load(w, r)
		if !ok {
			return
		}
		rng, f, ok := statsQuery(w, r)
		if !ok {
			return
		}

		sum, err := api.Stats.Summary(r.Context(), site.ID, rng, f)
		if err != nil {
			api.Log.Error("stats summary", "err", err)
			_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to load summary")
			return
		}
		out := summaryResponse{
			From:           rng.From.Format(time.RFC3339),
			To:             rng.To.Format(time.RFC3339),
			summaryMetrics: toMetrics(sum),
		}

		width := rng.To.Sub(rng.From)
		prev := stats.Range{From: rng.From.Add(-width), To: rng.From}
		if !f.Active() || !prev.From.Before(time.Now().Add(-stats.RawWindow)) {
			p, err := api.Stats.Summary(r.Context(), site.ID, prev, f)
			if err != nil {
				api.Log.Error("stats summary previous", "err", err)
				_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to load summary")
				return
			}
			m := toMetrics(p)
			out.Previous = &m
		}
		_ = writeJSON(w, http.StatusOK, out)
	}
}

func (api *API) timeseriesHandler(load siteLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := load(w, r)
		if !ok {
			return
		}
		rng, f, ok := statsQuery(w, r)
		if !ok {
			return
		}
		interval, err := stats.ParseInterval(r.URL.Query().Get("interval"), rng)
		if err != nil {
			_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		points, err := api.Stats.Timeseries(r.Context(), site.ID, rng, interval, f)
		if err != nil {
			api.Log.Error("stats timeseries", "err", err)
			_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to load timeseries")
			return
		}

		out := timeseriesResponse{Interval: string(interval), Points: make([]timeseriesPoint, 0, len(points))}
		for _, p := range points {
			out.Points = append(out.Points, timeseriesPoint{
				Bucket:    p.Bucket.UTC().Format(time.RFC3339),
				Pageviews: p.Pageviews,
				Visitors:  p.Visitors,
			})
		}
		_ = writeJSON(w, http.StatusOK, out)
	}
}

func (api *API) breakdownHandler(load siteLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := load(w, r)
		if !ok {
			return
		}
		rng, f, ok := statsQuery(w, r)
		if !ok {
			return
		}
		dim, err := stats.ParseDimension(r.URL.Query().Get("dimension"))
		if err != nil {
			_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		limit := int32(stats.DefaultBreakdownLimit)
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > stats.MaxBreakdownLimit {
				_ = writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
				return
			}
			limit = int32(n)
		}

		rows, err := api.Stats.Breakdown(r.Context(), site.ID, rng, dim, limit, f)
		if err != nil {
			api.Log.Error("stats breakdown", "err", err)
			_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to load breakdown")
			return
		}

		out := breakdownResponse{Dimension: string(dim), Rows: make([]breakdownRow, 0, len(rows))}
		for _, row := range rows {
			out.Rows = append(out.Rows, breakdownRow{Value: row.Value, Pageviews: row.Pageviews, Visitors: row.Visitors})
		}
		_ = writeJSON(w, http.StatusOK, out)
	}
}

func (api *API) realtimeHandler(load siteLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := load(w, r)
		if !ok {
			return
		}
		n, err := api.Stats.Realtime(r.Context(), site.ID)
		if err != nil {
			api.Log.Error("stats realtime", "err", err)
			_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to load realtime")
			return
		}
		_ = writeJSON(w, http.StatusOK, map[string]int64{"visitors": n})
	}
}
