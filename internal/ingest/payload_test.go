package ingest

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      Payload
		want    Event
		wantErr bool
	}{
		{
			name: "pageview with query and referrer",
			in: Payload{
				Domain:   "https://www.Example.com/",
				URL:      "https://example.com/pricing/?utm_source=HN&x=1#top",
				Referrer: "https://news.ycombinator.com/item?id=1",
				Width:    1440,
			},
			want: Event{
				Domain:    "example.com",
				Name:      "pageview",
				Pathname:  "/pricing",
				Referrer:  "news.ycombinator.com",
				UTMSource: "hn",
				Device:    "desktop",
			},
		},
		{
			name: "root path and same-site referrer",
			in: Payload{
				Domain:   "example.com",
				Name:     "signup",
				URL:      "https://example.com/",
				Referrer: "https://www.example.com/pricing",
				Width:    400,
			},
			want: Event{
				Domain:   "example.com",
				Name:     "signup",
				Pathname: "/",
				Device:   "mobile",
			},
		},
		{
			name: "no width means no device",
			in:   Payload{Domain: "example.com", URL: "https://example.com/a"},
			want: Event{Domain: "example.com", Name: "pageview", Pathname: "/a"},
		},
		{
			name:    "missing domain",
			in:      Payload{URL: "https://example.com/"},
			wantErr: true,
		},
		{
			name:    "missing url",
			in:      Payload{Domain: "example.com"},
			wantErr: true,
		},
		{
			name:    "non-http scheme",
			in:      Payload{Domain: "example.com", URL: "file:///etc/passwd"},
			wantErr: true,
		},
		{
			name:    "event name with spaces",
			in:      Payload{Domain: "example.com", Name: "sign up", URL: "https://example.com/"},
			wantErr: true,
		},
		{
			name: "path over the cap",
			in: Payload{
				Domain: "example.com",
				URL:    "https://example.com/" + strings.Repeat("a", MaxPathLen+1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse() error = nil, want an error (got %+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Parse() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}
