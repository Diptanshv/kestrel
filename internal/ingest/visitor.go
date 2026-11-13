package ingest

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the best-guess client address, preferring the left-most
// X-Forwarded-For entry because the API runs behind a proxy in production.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
