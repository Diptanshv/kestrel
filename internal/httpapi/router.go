package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(logger *slog.Logger, queries *store.Queries) http.Handler {
	api := &API{Log: logger, Q: queries}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/healthz", handleHealthz)
	r.Post("/api/auth/register", api.handleRegister)
	return r
}
