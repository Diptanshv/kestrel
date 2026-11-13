package ingest

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name string
		xff  []string
		hops int
		want string
	}{
		{"no proxy ignores header", []string{"1.1.1.1"}, 0, "10.0.0.1"},
		{"one hop takes the right-most entry", []string{"6.6.6.6, 2.2.2.2"}, 1, "2.2.2.2"},
		{"client-forged prefix is ignored", []string{"6.6.6.6", "7.7.7.7, 2.2.2.2"}, 1, "2.2.2.2"},
		{"two hops", []string{"2.2.2.2, 3.3.3.3"}, 2, "2.2.2.2"},
		{"too few entries falls back to the peer", nil, 1, "10.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/event", nil)
			r.RemoteAddr = "10.0.0.1:5555"
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := ClientIP(r, tc.hops); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
