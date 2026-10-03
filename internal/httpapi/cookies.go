package httpapi

import (
	"net/http"
	"time"

	"github.com/Diptanshv/kestrel/internal/auth"
)

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  expires,
	})
}

func readSessionCookie(r *http.Request) (string, error) {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return "", err
	}

	return auth.SessionTokenFromCookie(c.Value)
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}
