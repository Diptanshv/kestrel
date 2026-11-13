package ingest

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzParse checks the properties every accepted event must have, whatever
// the input: storable text, bounded lengths, and a canonical path.
//
//	go test ./internal/ingest -run '^$' -fuzz FuzzParse -fuzztime 60s
func FuzzParse(f *testing.F) {
	for _, seed := range []struct{ d, n, u, r string }{
		{"example.com", "pageview", "https://example.com/pricing/?utm_source=HN#x", "https://news.ycombinator.com/item?id=1"},
		{"Example.COM", "signup", "http://example.com", ""},
		{"example.com", "", "https://example.com/%00", ""},
		{"example.com", "", "https://example.com/%ff%fe", ""},
		{"example.com", "", "https://example.com/a?utm_source=%e2%82", "https://www.example.com/"},
		{"example.com", "", "https://example.com/" + strings.Repeat("a", 250), "javascript:alert(1)"},
	} {
		f.Add(seed.d, seed.n, seed.u, seed.r)
	}

	f.Fuzz(func(t *testing.T, d, n, u, r string) {
		ev, err := Parse(Payload{Domain: d, Name: n, URL: u, Referrer: r, Width: 1200})
		if err != nil {
			return
		}
		for field, v := range map[string]string{
			"domain": ev.Domain, "name": ev.Name, "path": ev.Pathname,
			"referrer": ev.Referrer, "utm": ev.UTMSource,
		} {
			if !utf8.ValidString(v) || !cleanText(v) {
				t.Fatalf("%s %q is not storable text", field, v)
			}
		}
		if !strings.HasPrefix(ev.Pathname, "/") {
			t.Fatalf("path %q does not start with /", ev.Pathname)
		}
		if len(ev.Pathname) > 1 && strings.HasSuffix(ev.Pathname, "/") {
			t.Fatalf("path %q keeps a trailing slash", ev.Pathname)
		}
		if strings.ContainsAny(ev.Pathname, "?#") && !strings.ContainsAny(u, "%") {
			t.Fatalf("path %q kept a query or fragment", ev.Pathname)
		}
		if len(ev.Pathname) > MaxPathLen || len(ev.Name) > MaxNameLen ||
			len(ev.Referrer) > MaxReferrerLen || len(ev.UTMSource) > MaxUTMSourceLen {
			t.Fatalf("length cap exceeded: %+v", ev)
		}
		// Normalising is idempotent: the stored path survives a round trip.
		again, err := normalizePath(ev.Pathname)
		if err != nil || again != ev.Pathname {
			t.Fatalf("normalizePath not idempotent: %q -> %q (%v)", ev.Pathname, again, err)
		}
	})
}
