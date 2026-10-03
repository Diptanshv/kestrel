package ingest

import (
	"crypto/sha256"
	"encoding/binary"
	"net"
	"net/http"
	"strings"
)

// VisitorIDLen matches the CHECK constraint on events_raw.visitor_id.
const VisitorIDLen = 16

// VisitorID is a PLACEHOLDER identity for Phase 3: an unsalted digest of
// site, IP and User-Agent. It has the right shape (16 bytes) but none of the
// privacy properties - without a rotating salt the same visitor hashes the
// same forever, so this is cross-day linkable.
//
// Phase 5 replaces the body with HMAC-SHA256 keyed by the daily salt. The
// signature stays the same so callers do not change.
//
// The IP is used here and never stored or logged.
func VisitorID(siteID int64, ip, userAgent string) []byte {
	h := sha256.New()
	var site [8]byte
	binary.BigEndian.PutUint64(site[:], uint64(siteID))
	h.Write(site[:])
	h.Write([]byte(ip))
	h.Write([]byte(userAgent))
	return h.Sum(nil)[:VisitorIDLen]
}

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
