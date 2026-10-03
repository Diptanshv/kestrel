package httpapi

import "time"

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
