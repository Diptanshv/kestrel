package sites

import "testing"

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "https URL with www and uppercase",
			input: "https://www.Example.com/",
			want:  "www.example.com",
		},
		{
			name:  "http URL",
			input: "http://example.com",
			want:  "example.com",
		},
		{
			name:  "domain without scheme",
			input: "example.com",
			want:  "example.com",
		},
		{
			name:  "domain with whitespace",
			input: "  example.com  ",
			want:  "example.com",
		},
		{
			name:    "empty domain",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			input:   "https://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeDomain(tt.input)

			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeDomain() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if got != tt.want {
				t.Errorf("NormalizeDomain() = %q, want %q", got, tt.want)
			}
		})
	}
}
