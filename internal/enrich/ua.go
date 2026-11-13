package enrich

import "github.com/mileusna/useragent"

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
	out := UA{Browser: ua.Name, OS: ua.OS, Bot: ua.Bot}
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
