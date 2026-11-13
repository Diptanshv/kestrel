package sites

import (
	"strings"
	"testing"
)

// FuzzNormalizeDomain: whatever goes in, what comes out is a canonical,
// idempotent, plain-ASCII hostname.
func FuzzNormalizeDomain(f *testing.F) {
	for _, s := range []string{
		"example.com", "https://www.Example.com/path?q=1", "  sub.example.co.uk  ",
		"localhost:8080", "http://[::1]:80", "exa%41mple.com", "exa mple.com",
		"münchen.de", "a..b", "-bad-.com", strings.Repeat("a", 300) + ".com",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out, err := NormalizeDomain(in)
		if err != nil {
			return
		}
		if again, err := NormalizeDomain(out); err != nil || again != out {
			t.Fatalf("not idempotent: %q -> %q -> %q (%v)", in, out, again, err)
		}
		if strings.HasPrefix(out, "www.") || out != strings.ToLower(out) {
			t.Fatalf("not canonical: %q", out)
		}
		if len(out) > 253 {
			t.Fatalf("too long: %d", len(out))
		}
		for i := 0; i < len(out); i++ {
			if out[i] != '.' && !isLabelByte(out[i]) {
				t.Fatalf("unexpected byte %q in %q", out[i], out)
			}
		}
	})
}
