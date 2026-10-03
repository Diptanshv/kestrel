package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/Diptanshv/kestrel/internal/auth"
	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// registerResponse is the JSON body for a successful POST /api/auth/register.
type registerResponse = userResponse

func (api *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	email, err := normaliseEmail(req.Email)
	if err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if err := validatePassword(req.Password); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		api.Log.Error("hash password", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to hash password")
		return
	}

	user, err := api.Q.CreateUser(r.Context(), store.CreateUserParams{
		Email:        email,
		PasswordHash: hashedPassword,
	})

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_ = writeError(w, http.StatusConflict, "email_taken", "email already exists")
			return
		}
		api.Log.Error("create user", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to create user")
		return
	}

	response := userToResponse(user)

	_ = writeJSON(w, http.StatusCreated, response)
}

func userToResponse(u store.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

func (api *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}

	email, err := normaliseEmail(req.Email)
	if err != nil {
		_ = writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	user, err := api.Q.GetUserByEmail(r.Context(), email)
	if err != nil {
		_ = writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		_ = writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}

	token, err := auth.NewSessionToken()
	if err != nil {
		api.Log.Error("session token", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to create session")
		return
	}

	expires := time.Now().UTC().Add(api.Config.SessionTTL)
	hash := auth.HashSessionToken(api.Config.SessionSecret, token)

	_, err = api.Q.CreateSession(r.Context(), store.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: expires,
	})
	if err != nil {
		api.Log.Error("create session", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
		return
	}

	csrf, err := newCSRFToken()
	if err != nil {
		api.Log.Error("csrf token", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
		return
	}

	setSessionCookie(w, token, expires, api.Config.CookieSecure)
	setCSRFCookie(w, csrf, api.Config.CookieSecure)
	_ = writeJSON(w, http.StatusOK, userToResponse(user))
}

func (api *API) handleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		_ = writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	user, err := api.Q.GetUserByID(r.Context(), userID)
	if err != nil {
		api.Log.Error("get user", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to load user")
		return
	}

	_ = writeJSON(w, http.StatusOK, userToResponse(user))
}

func (api *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	token, err := readSessionCookie(r)
	if err == nil {
		hash := auth.HashSessionToken(api.Config.SessionSecret, token)
		if err := api.Q.DeleteSessionByTokenHash(r.Context(), hash); err != nil {
			api.Log.Error("delete session", "err", err)
		}
	}

	clearSessionCookie(w, api.Config.CookieSecure)
	clearCSRFCookie(w, api.Config.CookieSecure)
	w.WriteHeader(http.StatusNoContent)
}
