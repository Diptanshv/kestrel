package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

const csrfCookieName = "csrf_token"
const csrfHeaderName = "X-CSRF-Token"

func newCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func setCSRFCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // so that our dashboard can read this; double-submit pattern
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func clearCSRFCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func (api *API) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(csrfCookieName)
		if err != nil || c.Value == "" {
			_ = writeError(w, http.StatusForbidden, "forbidden", "missing csrf token")
			return
		}

		if r.Header.Get(csrfHeaderName) != c.Value {
			_ = writeError(w, http.StatusForbidden, "forbidden", "invalid csrf token")
			return
		}

		next.ServeHTTP(w, r)
	})
}
