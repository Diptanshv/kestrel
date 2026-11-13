package httpapi

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Diptanshv/kestrel/internal/ingest"
	"github.com/Diptanshv/kestrel/internal/metrics"
	"github.com/Diptanshv/kestrel/internal/ratelimit"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			logger.Info("request",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// observeDuration records latency by chi route pattern rather than raw path,
// so /api/sites/{id}/summary is one series instead of one per site.
func observeDuration(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil {
			if p := rc.RoutePattern(); p != "" {
				route = p
			}
		}
		metrics.HTTPDuration.
			WithLabelValues(r.Method, route, strconv.Itoa(ww.Status())).
			Observe(time.Since(start).Seconds())
	})
}

// allowTrackerCORS echoes the request Origin so the tracker's fetch fallback
// can read the response. sendBeacon ignores the response entirely, and the
// text/plain body keeps the POST preflight-free, so this mostly matters for
// the fallback path.
func allowTrackerCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func handleEventPreflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// securityHeaders sets conservative defaults on every response. The API
// serves JSON and one script, never HTML that should be framed or sniffed.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// requireMetricsToken hides /metrics unless a bearer token is configured and
// presented. Metrics leak traffic volumes and internal timings.
func (api *API) requireMetricsToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := api.Config.MetricsToken
		if want == "" {
			_ = writeError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			_ = writeError(w, http.StatusUnauthorized, "unauthorized", "invalid metrics token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// limitAuth throttles login and registration per client IP, which bounds
// online password guessing to the limiter's rate.
func (api *API) limitAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ingest.ClientIP(r, api.Config.TrustedProxyHops)
		if !allow(api.Limits.Auth, ratelimit.IPKey(ip)) {
			w.Header().Set("Retry-After", "60")
			_ = writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}
