package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Diptanshv/kestrel/internal/batch"
	"github.com/Diptanshv/kestrel/internal/ingest"
	"github.com/Diptanshv/kestrel/internal/metrics"
	"github.com/Diptanshv/kestrel/internal/ratelimit"
)

// handleEvent validates and enriches in the request, then hands the row to the
// sink and returns. No database write happens on this path; the batcher
// flushes within FlushInterval.
func (api *API) handleEvent(w http.ResponseWriter, r *http.Request) {
	ip := ingest.ClientIP(r, api.Config.TrustedProxyHops)
	// The per-IP check comes first so a flood is turned away before it costs
	// a JSON parse or a site lookup.
	if !allow(api.Limits.IngestIP, ratelimit.IPKey(ip)) {
		rateLimited(w)
		return
	}

	var p ingest.Payload
	r.Body = http.MaxBytesReader(w, r.Body, ingest.MaxBodyBytes)
	if err := decodeJSON(w, r, &p); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			_ = writeError(w, http.StatusRequestEntityTooLarge, "too_large", "event body too large")
			return
		}
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	ev, err := ingest.Parse(p)
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	site, err := api.sites.get(r.Context(), ev.Domain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = writeError(w, http.StatusNotFound, "not_found", "unknown site")
			return
		}
		api.Log.Error("lookup site by domain", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to record event")
		return
	}

	if !originMatchesDomain(r.Header.Get("Origin"), site.Domain) {
		_ = writeError(w, http.StatusForbidden, "forbidden", "origin does not match site domain")
		return
	}

	if !allow(api.Limits.IngestSite, strconv.FormatInt(site.ID, 10)) {
		rateLimited(w)
		return
	}

	f, err := api.Enrich.Enrich(r.Context(), site.ID, ip, r.UserAgent(), time.Now())
	if err != nil {
		api.Log.Error("enrich event", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to record event")
		return
	}
	if f.Bot {
		metrics.EventsDropped.WithLabelValues(metrics.DropBot).Inc()
		// Accepted and dropped. A 4xx would tell the bot it was recognised, and
		// this is not the client's error anyway.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// The UA is authoritative for device; the tracker's viewport width is the
	// fallback for user agents the parser does not classify.
	device := f.Device
	if device == "" {
		device = ev.Device
	}

	// A full buffer still answers 202: the tracker cannot do anything useful
	// with an error, and the drop is counted in events_dropped_total.
	api.Sink.Enqueue(batch.Event{
		SiteID:    site.ID,
		Ts:        time.Now().UTC(),
		VisitorID: f.VisitorID,
		SessionID: f.SessionID,
		Name:      ev.Name,
		Pathname:  ev.Pathname,
		Referrer:  nullString(ev.Referrer),
		UtmSource: nullString(ev.UTMSource),
		Country:   nullString(f.Country),
		Browser:   nullString(f.Browser),
		Os:        nullString(f.OS),
		Device:    nullString(device),
	})

	w.WriteHeader(http.StatusAccepted)
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func allow(l *ratelimit.Limiter, key string) bool {
	return l == nil || l.Allow(key)
}

// rateLimited answers 429. The tracker ignores responses, so this is for
// scripted clients; the drop is counted either way.
func rateLimited(w http.ResponseWriter) {
	metrics.EventsDropped.WithLabelValues(metrics.DropRateLimit).Inc()
	w.Header().Set("Retry-After", "1")
	_ = writeError(w, http.StatusTooManyRequests, "rate_limited", "too many events")
}
