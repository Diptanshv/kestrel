package enrich

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// VisitorIDLen matches the CHECK constraint on events_raw.visitor_id.
const VisitorIDLen = 16

// VisitorID is first16(HMAC-SHA256(salt, site || ip || user_agent)).
//
// The IP and user agent are used here and nowhere else - neither is returned,
// stored or logged. Once the day's salt is deleted the output cannot be linked
// back to either, which is the whole privacy model in one function.
//
// Fields are separated by a zero byte so that different inputs cannot
// concatenate into the same message: without it, site 1 with IP "23..." and
// site 12 with IP "3..." would hash identically.
func VisitorID(salt []byte, siteID int64, ip, userAgent string) []byte {
	mac := hmac.New(sha256.New, salt)

	var site [8]byte
	binary.BigEndian.PutUint64(site[:], uint64(siteID))
	mac.Write(site[:])
	mac.Write([]byte{0})
	mac.Write([]byte(ip))
	mac.Write([]byte{0})
	mac.Write([]byte(userAgent))

	return mac.Sum(nil)[:VisitorIDLen]
}
