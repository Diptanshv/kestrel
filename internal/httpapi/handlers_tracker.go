package httpapi

import (
	"bytes"
	"net/http"
	"time"

	"github.com/Diptanshv/kestrel/internal/tracker"
)

// buildTime is the modtime reported for the embedded assets. A zero time means
// ServeContent skips Last-Modified and relies on the ETag it derives from the
// content, which is what we want for an embedded file.
var buildTime time.Time

func handleScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// One hour for now. Phase 6 should move to a versioned filename
	// (/script.v2.js) plus immutable, year-long caching.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeContent(w, r, "script.js", buildTime, bytes.NewReader(tracker.Script))
}

// handleDemo serves the local test page. Off unless ENABLE_DEMO=true, so it
// never ships in production.
func (api *API) handleDemo(w http.ResponseWriter, r *http.Request) {
	if !api.Config.EnableDemo {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "demo.html", buildTime, bytes.NewReader(tracker.Demo))
}
