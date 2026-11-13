package ingest

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the client address. trustedHops is how many reverse
// proxies we run behind: each appends the address it received the request
// from to X-Forwarded-For, so the client is the entry trustedHops from the
// right. Everything left of that was written by the client and is ignored -
// trusting the left-most entry would let anyone pick their own IP, defeating
// per-IP rate limits and minting unlimited "unique visitors".
//
// With trustedHops 0 the header is ignored entirely.
func ClientIP(r *http.Request, trustedHops int) string {
	if trustedHops > 0 {
		var hops []string
		for _, h := range r.Header.Values("X-Forwarded-For") {
			for _, part := range strings.Split(h, ",") {
				if p := strings.TrimSpace(part); p != "" {
					hops = append(hops, p)
				}
			}
		}
		if len(hops) >= trustedHops {
			return hops[len(hops)-trustedHops]
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
