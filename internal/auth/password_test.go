package auth

import "testing"

func TestPassword(t *testing.T) {
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

	if err := VerifyPassword("secret", hash); err != nil {
		t.Fatalf("VerifyPassword() with correct password error = %v", err)
	}

	if err := VerifyPassword("wrong", hash); err == nil {
		t.Fatal("VerifyPassword() with wrong password expected an error")
	}
}
