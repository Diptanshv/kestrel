package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Diptanshv/kestrel/internal/sites"
	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type createSiteRequest struct {
	Domain string `json:"domain"`
}

type patchSiteRequest struct {
	Domain     *string `json:"domain"`
	PublicSlug *string `json:"public_slug"`
}

type siteResponse struct {
	ID         int64   `json:"id"`
	Domain     string  `json:"domain"`
	PublicSlug *string `json:"public_slug"`
	CreatedAt  string  `json:"created_at"`
}

func siteToResponse(s store.Site) siteResponse {
	var slug *string
	if s.PublicSlug.Valid {
		slug = &s.PublicSlug.String
	}
	return siteResponse{
		ID:         s.ID,
		Domain:     s.Domain,
		PublicSlug: slug,
		CreatedAt:  s.CreatedAt.Format(time.RFC3339),
	}
}

func (api *API) handleListSites(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	rows, err := api.Q.ListSitesByUserID(r.Context(), userID)
	if err != nil {
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to list sites")
		return
	}
	out := make([]siteResponse, 0, len(rows))
	for _, s := range rows {
		out = append(out, siteToResponse(s))
	}
	_ = writeJSON(w, http.StatusOK, out)
}

func (api *API) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	var req createSiteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	domain, err := sites.NormalizeDomain(req.Domain)
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	site, err := api.Q.CreateSite(r.Context(), store.CreateSiteParams{
		UserID:     userID,
		Domain:     domain,
		PublicSlug: sql.NullString{}, // invalid = NULL
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_ = writeError(w, http.StatusConflict, "conflict", "domain already registered")
			return
		}
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to create site")
		return
	}
	_ = writeJSON(w, http.StatusCreated, siteToResponse(site))
}

func (api *API) loadSiteForOwner(w http.ResponseWriter, r *http.Request) (store.Site, bool) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		_ = writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return store.Site{}, false
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "invalid site id")
		return store.Site{}, false
	}
	site, err := api.Q.GetSiteByID(r.Context(), id)
	if err != nil {
		_ = writeError(w, http.StatusNotFound, "not_found", "site not found")
		return store.Site{}, false
	}
	if site.UserID != userID {
		_ = writeError(w, http.StatusForbidden, "forbidden", "not your site")
		return store.Site{}, false
	}
	return site, true
}

func (api *API) handleGetSite(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}
	_ = writeJSON(w, http.StatusOK, siteToResponse(site))
}

func (api *API) handlePatchSite(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}

	var req patchSiteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.Domain == nil && req.PublicSlug == nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "no fields to update")
		return
	}

	params := store.UpdateSiteParams{ID: site.ID}
	if req.Domain != nil {
		domain, err := sites.NormalizeDomain(*req.Domain)
		if err != nil {
			_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		params.Domain = sql.NullString{String: domain, Valid: true}
	}
	if req.PublicSlug != nil {
		params.PublicSlug = sql.NullString{String: *req.PublicSlug, Valid: true}
	}

	updated, err := api.Q.UpdateSite(r.Context(), params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_ = writeError(w, http.StatusConflict, "conflict", "domain or public slug already in use")
			return
		}
		api.Log.Error("update site", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to update site")
		return
	}

	_ = writeJSON(w, http.StatusOK, siteToResponse(updated))
}

func (api *API) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}

	if err := api.Q.DeleteSite(r.Context(), site.ID); err != nil {
		api.Log.Error("delete site", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to delete site")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
