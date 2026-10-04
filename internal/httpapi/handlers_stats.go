package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Diptanshv/kestrel/internal/stats"
)

type summaryResponse struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Pageviews int64  `json:"pageviews"`
	Visitors  int64  `json:"visitors"`
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

// statsRange pulls from/to off the query string, reporting the error to the
// client itself. The bool is false when the response has already been written.
func statsRange(w http.ResponseWriter, r *http.Request) (stats.Range, bool) {
	q := r.URL.Query()
	rng, err := stats.ParseRange(q.Get("from"), q.Get("to"), time.Now())
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return stats.Range{}, false
	}
	return rng, true
}

func (api *API) handleSiteSummary(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}
	rng, ok := statsRange(w, r)
	if !ok {
		return
	}

	sum, err := api.Stats.Summary(r.Context(), site.ID, rng)
	if err != nil {
		api.Log.Error("stats summary", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to load summary")
		return
	}

	_ = writeJSON(w, http.StatusOK, summaryResponse{
		From:      rng.From.Format(time.RFC3339),
		To:        rng.To.Format(time.RFC3339),
		Pageviews: sum.Pageviews,
		Visitors:  sum.Visitors,
	})
}

func (api *API) handleSiteTimeseries(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}
	rng, ok := statsRange(w, r)
	if !ok {
		return
	}

	interval, err := stats.ParseInterval(r.URL.Query().Get("interval"), rng)
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	points, err := api.Stats.Timeseries(r.Context(), site.ID, rng, interval)
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

func (api *API) handleSiteBreakdown(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}
	rng, ok := statsRange(w, r)
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

	rows, err := api.Stats.Breakdown(r.Context(), site.ID, rng, dim, limit)
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
