package httpapi

import (
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  string
	}{
		{name: "empty", password: "", wantErr: "password is required"},
		{name: "too short", password: "short", wantErr: "password must be at least 8 characters long"},
		{name: "min length", password: "12345678", wantErr: ""},
		{name: "longer", password: "correct-horse-battery-staple", wantErr: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePassword(tt.password)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validatePassword() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validatePassword() expected an error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("validatePassword() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNormaliseEmail(t *testing.T) {
	t.Parallel()

	got, err := normaliseEmail("  User@Example.COM  ")
	if err != nil {
		t.Fatalf("normaliseEmail() error = %v", err)
	}
	if got != "user@example.com" {
		t.Fatalf("normaliseEmail() = %q, want %q", got, "user@example.com")
	}

	_, err = normaliseEmail("")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("normaliseEmail(\"\") error = %v, want required", err)
	}

	_, err = normaliseEmail("not-an-email")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("normaliseEmail(invalid) error = %v, want invalid", err)
	}
}
