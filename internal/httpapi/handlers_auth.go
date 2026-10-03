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
		if errors.As(err, &pgErr) && pgErr.Code == "2305" {
			_ = writeError(w, http.StatusConflict, "email_taken", "email already exists")
			return
		}
		api.Log.Error("create user", "err", err)
		_ = writeError(w, http.StatusInternalServerError, "internal_server_error", "failed to create user")
		return
	}

	response := registerResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	}

	_ = writeJSON(w, http.StatusCreated, response)
}
