package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const SessionCookieName = "session"

func NewSessionToken() (string, error) {

	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashSessionToken(secret, token string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return mac.Sum(nil)
}

func SessionTokenFromCookie(cookieValue string) (string, error) {
	if cookieValue == "" {
		return "", fmt.Errorf("empty session cookie")
	}
	return cookieValue, nil
}
