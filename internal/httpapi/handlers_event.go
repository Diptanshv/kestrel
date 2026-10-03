package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/Diptanshv/kestrel/internal/ingest"
	"github.com/Diptanshv/kestrel/internal/store"
)

// unassignedSession is written to events_raw.session_id until Phase 5 adds the
// in-memory session tracker. The column is NOT NULL and has no table behind it.
const unassignedSession int64 = 0

// handleEvent inserts synchronously, on purpose. Phase 6 replaces this with a
// buffered channel and a batcher, and benchmarks the two against each other.
func (api *API) handleEvent(w http.ResponseWriter, r *http.Request) {
	var p ingest.Payload
	if err := decodeJSON(w, r, &p); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	ev, err := ingest.Parse(p)
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	site, err := api.Q.GetSiteByDomain(r.Context(), ev.Domain)
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

	_, err = api.Q.CreateEventRaw(r.Context(), store.CreateEventRawParams{
		SiteID:    site.ID,
		Ts:        time.Now().UTC(),
		VisitorID: ingest.VisitorID(site.ID, ingest.ClientIP(r), r.UserAgent()),
		SessionID: unassignedSession,
		Name:      ev.Name,
		Pathname:  ev.Pathname,
		Referrer:  nullString(ev.Referrer),
		UtmSource: nullString(ev.UTMSource),
		Country:   sql.NullString{}, // Phase 5: GeoIP
		Browser:   sql.NullString{}, // Phase 5: UA parsing
		Os:        sql.NullString{}, // Phase 5: UA parsing
		Device:    nullString(ev.Device),
	})
	if err != nil {
		api.Log.Error("insert event", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to record event")
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
