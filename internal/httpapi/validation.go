package httpapi

import (
	"errors"
	"net/mail"
	"strings"
)

func normaliseEmail(email string) (string, error) {
	if email == "" {
		return "", errors.New("email is required")
	}
	email = strings.TrimSpace(email)
	email = strings.ToLower(email)

	if _, err := mail.ParseAddress(email); err != nil {
		return "", errors.New("invalid email address")
	}
	return email, nil
}

func validatePassword(password string) error {
	if password == "" {
		return errors.New("password is required")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}
	return nil
}
