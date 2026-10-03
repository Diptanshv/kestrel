package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/Diptanshv/kestrel/internal/auth"
)

func (api *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := readSessionCookie(r)
		if err != nil {
			_ = writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
			return
		}

		hash := auth.HashSessionToken(api.Config.SessionSecret, token)
		sess, err := api.Q.GetSessionByTokenHash(r.Context(), hash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				_ = writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
				return
			}
			api.Log.Error("get session", "err", err)
			_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
			return
		}

		r = r.WithContext(withUserID(r.Context(), sess.UserID))
		next.ServeHTTP(w, r)

	})
}
