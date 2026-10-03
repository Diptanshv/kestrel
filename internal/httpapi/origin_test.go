package httpapi

import "testing"

func TestOriginMatchesDomain(t *testing.T) {
	tests := []struct {
		origin string
		domain string
		want   bool
	}{
		{"https://example.com", "example.com", true},
		{"https://www.example.com", "example.com", true},
		{"https://example.com", "www.example.com", true},
		{"https://blog.example.com", "example.com", true},
		{"http://localhost:8080", "localhost", true},
		{"https://evil.com", "example.com", false},
		{"https://notexample.com", "example.com", false},
		{"https://example.com.evil.com", "example.com", false},
		{"", "example.com", false},
		{"null", "example.com", false},
		{"https://example.com", "", false},
	}

	for _, tt := range tests {
		if got := originMatchesDomain(tt.origin, tt.domain); got != tt.want {
			t.Errorf("originMatchesDomain(%q, %q) = %v, want %v", tt.origin, tt.domain, got, tt.want)
		}
	}
}
