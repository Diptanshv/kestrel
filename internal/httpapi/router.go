package httpapi

import (
	"net/http"

	"github.com/Diptanshv/kestrel/internal/stats"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(d Deps) http.Handler {
	api := &API{
		Log:    d.Log,
		Q:      d.Queries,
		Config: d.Config,
		Stats:  stats.New(d.Queries, d.Config.Timezone),
		Enrich: d.Enrich,
		Sink:   d.Sink,
		Rollup: d.Rollup,
		sites:  newSiteCache(d.Queries),
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(d.Log))
	r.Use(observeDuration)

	r.Get("/healthz", handleHealthz)
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/script.js", handleScript)
	r.Get("/demo.html", api.handleDemo)
	r.Post("/internal/rollup", api.handleRollup)
	r.Post("/api/auth/register", api.handleRegister)
	r.Post("/api/auth/login", api.handleLogin)

	// Public ingest: any origin may post, but the Origin must match the
	// site's registered domain (checked in the handler).
	r.Group(func(r chi.Router) {
		r.Use(allowTrackerCORS)
		r.Post("/api/event", api.handleEvent)
		r.Options("/api/event", handleEventPreflight)
	})

	r.Group(func(r chi.Router) {
		r.Use(api.requireAuth)
		r.Use(api.requireCSRF)
		r.Get("/api/auth/me", api.handleMe)
		r.Post("/api/auth/logout", api.handleLogout)
		r.Get("/api/sites", api.handleListSites)
		r.Post("/api/sites", api.handleCreateSite)
		r.Get("/api/sites/{id}", api.handleGetSite)
		r.Patch("/api/sites/{id}", api.handlePatchSite)
		r.Delete("/api/sites/{id}", api.handleDeleteSite)
		r.Get("/api/sites/{id}/summary", api.summaryHandler(api.loadSiteForOwner))
		r.Get("/api/sites/{id}/timeseries", api.timeseriesHandler(api.loadSiteForOwner))
		r.Get("/api/sites/{id}/breakdown", api.breakdownHandler(api.loadSiteForOwner))
		r.Get("/api/sites/{id}/realtime", api.realtimeHandler(api.loadSiteForOwner))
		r.Post("/api/sites/{id}/share", api.handleEnableShare)
		r.Delete("/api/sites/{id}/share", api.handleDisableShare)
	})

	// Shared dashboards: read-only, no session. The slug is the credential.
	r.Route("/api/public/{slug}", func(r chi.Router) {
		r.Get("/", api.handlePublicSite)
		r.Get("/summary", api.summaryHandler(api.loadSiteBySlug))
		r.Get("/timeseries", api.timeseriesHandler(api.loadSiteBySlug))
		r.Get("/breakdown", api.breakdownHandler(api.loadSiteBySlug))
		r.Get("/realtime", api.realtimeHandler(api.loadSiteBySlug))
	})

	return r
}
