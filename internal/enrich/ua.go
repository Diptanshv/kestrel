package enrich

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mileusna/useragent"
)

// maxLabelLen caps browser and OS names. Unknown browsers come through as
// whatever token the client sent, so the label is attacker-controlled.
const maxLabelLen = 32

// UA is what we keep from a user agent string. The string itself is never
// stored - only these three coarse labels.
type UA struct {
	Browser string
	OS      string
	Device  string // "desktop" | "mobile" | "tablet" | ""
	Bot     bool
}

// ParseUA extracts the coarse labels the dashboard breaks down by.
func ParseUA(s string) UA {
	// A request with no user agent at all is not a browser. Real traffic always
	// carries one, so this costs nothing and catches the laziest scripts.
	if s == "" {
		return UA{Bot: true}
	}

	ua := useragent.Parse(s)
	out := UA{Browser: label(ua.Name), OS: label(ua.OS), Bot: ua.Bot}
	switch {
	case ua.Tablet:
		out.Device = "tablet"
	case ua.Mobile:
		out.Device = "mobile"
	case ua.Desktop:
		out.Device = "desktop"
	}
	return out
}

// label makes a parser-derived name safe to store: valid UTF-8, no control
// characters, bounded length. Anything else is dropped to "".
func label(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLabelLen || !utf8.ValidString(s) {
		return ""
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return s
}
