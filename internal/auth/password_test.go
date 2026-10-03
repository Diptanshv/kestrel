package auth

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword() returned an empty hash")
	}
	if hash == "secret" {
		t.Fatal("HashPassword() returned the plain-text password")
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("HashPassword() hash = %q, expected argon2id encoding", hash)
	}
}

func TestHashPasswordDistinctHashes(t *testing.T) {
	t.Parallel()

	const password = "same-password"
	h1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("first HashPassword() error = %v", err)
	}
	h2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("second HashPassword() error = %v", err)
	}
	if h1 == h2 {
		t.Fatal("HashPassword() returned identical hashes for the same password")
	}
}

func TestVerifyPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if err := VerifyPassword("correct-horse-battery-staple", hash); err != nil {
		t.Fatalf("VerifyPassword() with correct password error = %v", err)
	}
	if err := VerifyPassword("wrong", hash); err == nil {
		t.Fatal("VerifyPassword() with wrong password expected an error")
	}
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	t.Parallel()

	err := VerifyPassword("secret", "not-a-valid-hash")
	if err == nil {
		t.Fatal("VerifyPassword() with invalid hash expected an error")
	}
}
