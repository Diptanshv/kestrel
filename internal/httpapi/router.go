package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(logger *slog.Logger, queries *store.Queries, cfg config.Config) http.Handler {
	api := &API{Log: logger, Q: queries, Config: cfg}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/healthz", handleHealthz)
	r.Post("/api/auth/register", api.handleRegister)
	r.Post("/api/auth/login", api.handleLogin)

	r.Group(func(r chi.Router) {
		r.Use(api.requireAuth)
		r.Get("/api/auth/me", api.handleMe)
		r.Post("/api/auth/logout", api.handleLogut)
	})

	return r
}
