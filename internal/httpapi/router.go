package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/Diptanshv/kestrel/internal/stats"
	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(logger *slog.Logger, queries *store.Queries, cfg config.Config) http.Handler {
	api := &API{Log: logger, Q: queries, Config: cfg, Stats: stats.New(queries, cfg.Timezone)}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/healthz", handleHealthz)
	r.Get("/script.js", handleScript)
	r.Get("/demo.html", api.handleDemo)
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
		r.Get("/api/sites/{id}/summary", api.handleSiteSummary)
		r.Get("/api/sites/{id}/timeseries", api.handleSiteTimeseries)
		r.Get("/api/sites/{id}/breakdown", api.handleSiteBreakdown)
	})

	return r
}
