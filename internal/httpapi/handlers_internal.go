package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/Diptanshv/kestrel/internal/rollup"
)

// handleRollup lets an external scheduler (GitHub Actions cron) trigger the
// rollup job, which matters on hosts that sleep and stop the in-process
// ticker. It is safe to call at any rate: the job is idempotent and guarded
// by an advisory lock.
func (api *API) handleRollup(w http.ResponseWriter, r *http.Request) {
	secret := api.Config.RollupSecret
	if secret == "" || api.Rollup == nil {
		// Unconfigured: pretend the route does not exist.
		_ = writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		_ = writeError(w, http.StatusUnauthorized, "unauthorized", "invalid rollup token")
		return
	}

	res, err := api.Rollup.Run(r.Context())
	if errors.Is(err, rollup.ErrBusy) {
		_ = writeError(w, http.StatusConflict, "busy", "rollup already running")
		return
	}
	if err != nil {
		api.Log.Error("rollup via http", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "rollup failed")
		return
	}
	_ = writeJSON(w, http.StatusOK, res)
}
