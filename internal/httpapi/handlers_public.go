package httpapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"net/http"
	"strings"

	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/go-chi/chi/v5"
)

// publicSiteResponse is all a shared dashboard reveals about its site.
type publicSiteResponse struct {
	Domain string `json:"domain"`
}

// loadSiteBySlug is the siteLoader for /api/public/{slug}/...: no session,
// and an unknown or revoked slug is a plain 404.
func (api *API) loadSiteBySlug(w http.ResponseWriter, r *http.Request) (store.Site, bool) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		_ = writeError(w, http.StatusNotFound, "not_found", "dashboard not found")
		return store.Site{}, false
	}
	site, err := api.Q.GetSiteByPublicSlug(r.Context(), sql.NullString{String: slug, Valid: true})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			api.Log.Error("load public site", "err", err)
		}
		_ = writeError(w, http.StatusNotFound, "not_found", "dashboard not found")
		return store.Site{}, false
	}
	return site, true
}

func (api *API) handlePublicSite(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteBySlug(w, r)
	if !ok {
		return
	}
	_ = writeJSON(w, http.StatusOK, publicSiteResponse{Domain: site.Domain})
}

// handleEnableShare turns the public dashboard on. The slug is random and
// unguessable rather than derived from the domain, so turning sharing off
// and on again issues a new link and kills the old one.
func (api *API) handleEnableShare(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}
	if site.PublicSlug.Valid {
		_ = writeJSON(w, http.StatusOK, siteToResponse(site))
		return
	}
	slug, err := newSlug()
	if err != nil {
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to create link")
		return
	}
	updated, err := api.Q.SetSitePublicSlug(r.Context(), store.SetSitePublicSlugParams{
		ID: site.ID, PublicSlug: sql.NullString{String: slug, Valid: true},
	})
	if err != nil {
		api.Log.Error("enable share", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to create link")
		return
	}
	_ = writeJSON(w, http.StatusOK, siteToResponse(updated))
}

func (api *API) handleDisableShare(w http.ResponseWriter, r *http.Request) {
	site, ok := api.loadSiteForOwner(w, r)
	if !ok {
		return
	}
	updated, err := api.Q.SetSitePublicSlug(r.Context(), store.SetSitePublicSlugParams{ID: site.ID})
	if err != nil {
		api.Log.Error("disable share", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to remove link")
		return
	}
	_ = writeJSON(w, http.StatusOK, siteToResponse(updated))
}

// newSlug returns 20 random characters (100 bits) from a URL-safe alphabet.
func newSlug() (string, error) {
	b := make([]byte, 13)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return strings.ToLower(s[:20]), nil
}
