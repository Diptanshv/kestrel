package httpapi

import (
	"net/url"
	"strings"
)

// originMatchesDomain reports whether an Origin header belongs to the site's
// registered domain. Subdomains are allowed and "www." is ignored on both
// sides. An empty or "null" Origin never matches: a browser always sends one
// for a cross-origin POST, so a missing Origin means a non-browser client.
//
// This stops casual spoofing only. Any HTTP client can set the header, so it
// is not a security boundary - rate limiting (Phase 5) is what bounds abuse.
func originMatchesDomain(origin, domain string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	domain = strings.TrimPrefix(strings.ToLower(domain), "www.")
	if domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}
