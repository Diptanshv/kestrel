package ingest

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Diptanshv/kestrel/internal/sites"
)

const (
	MaxPathLen      = 200
	MaxNameLen      = 64
	MaxReferrerLen  = 200
	MaxUTMSourceLen = 64
	MaxURLLen       = 2048
	MaxProps        = 10
	MaxPropLen      = 100

	// MaxBodyBytes caps the beacon body. A real one is ~200 bytes.
	MaxBodyBytes = 4 << 10
)

// Payload is the JSON body the tracker sends to POST /api/event.
// Field names are single letters to keep the beacon small.
type Payload struct {
	Domain   string            `json:"d"`
	Name     string            `json:"n"`
	URL      string            `json:"u"`
	Referrer string            `json:"r"`
	Width    int               `json:"w"`
	Props    map[string]string `json:"p"`
}

// Event is a validated, normalized payload ready to be stored.
type Event struct {
	Domain    string
	Name      string
	Pathname  string
	Referrer  string // host only; "" when absent or same-site
	UTMSource string
	Device    string // "" when the tracker sent no usable width
}

// Parse validates a tracker payload and normalizes it into an Event.
// It never returns a partially filled Event.
func Parse(p Payload) (Event, error) {
	domain, err := sites.NormalizeDomain(p.Domain)
	if err != nil {
		return Event{}, errors.New("invalid domain")
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "pageview"
	}
	if len(name) > MaxNameLen {
		return Event{}, errors.New("event name too long")
	}
	if !validEventName(name) {
		return Event{}, errors.New("invalid event name")
	}

	if strings.TrimSpace(p.URL) == "" {
		return Event{}, errors.New("url is required")
	}
	if len(p.URL) > MaxURLLen || len(p.Referrer) > MaxURLLen {
		return Event{}, errors.New("url too long")
	}
	if len(p.Props) > MaxProps {
		return Event{}, errors.New("too many props")
	}
	for k, v := range p.Props {
		if len(k) > MaxPropLen || len(v) > MaxPropLen {
			return Event{}, errors.New("prop too long")
		}
	}
	u, err := url.Parse(p.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Event{}, errors.New("invalid url")
	}

	path, err := normalizePath(u.Path)
	if err != nil {
		return Event{}, err
	}
	// url.Parse decodes percent-escapes, so the path can now hold bytes
	// Postgres refuses (NUL, invalid UTF-8). One such row would fail the
	// whole COPY batch it lands in, so it is rejected here.
	if !cleanText(path) {
		return Event{}, errors.New("invalid characters in path")
	}

	return Event{
		Domain:    domain,
		Name:      name,
		Pathname:  path,
		Referrer:  referrerHost(p.Referrer, u.Hostname()),
		UTMSource: utmSource(u),
		Device:    deviceFromWidth(p.Width),
	}, nil
}

// normalizePath drops the query and fragment (url.Parse already split them),
// collapses a trailing slash, and rejects anything over the length cap so one
// site cannot blow up path cardinality.
func normalizePath(path string) (string, error) {
	if path == "" {
		return "/", nil
	}
	if len(path) > MaxPathLen {
		return "", errors.New("pathname too long")
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
		if path == "" {
			return "/", nil
		}
	}
	return path, nil
}

// referrerHost keeps the host only, per the privacy model, and treats a
// same-site referrer as no referrer. "www." is stripped from both sides.
func referrerHost(referrer, selfHost string) string {
	referrer = strings.TrimSpace(referrer)
	if referrer == "" {
		return ""
	}
	ru, err := url.Parse(referrer)
	if err != nil || ru.Hostname() == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(ru.Hostname()), "www.")
	if host == strings.TrimPrefix(strings.ToLower(selfHost), "www.") {
		return ""
	}
	if len(host) > MaxReferrerLen || !cleanText(host) {
		return ""
	}
	return host
}

// utmSource is the one query parameter on the allowlist for now.
func utmSource(u *url.URL) string {
	v := strings.ToLower(strings.TrimSpace(u.Query().Get("utm_source")))
	if len(v) > MaxUTMSourceLen {
		v = v[:MaxUTMSourceLen]
	}
	// Cutting at a byte offset can split a rune; drop the value rather than
	// store something Postgres will reject.
	if !cleanText(v) {
		return ""
	}
	return v
}

// deviceFromWidth is a stand-in until Phase 5 parses the User-Agent.
func deviceFromWidth(width int) string {
	switch {
	case width <= 0:
		return ""
	case width < 768:
		return "mobile"
	case width < 1024:
		return "tablet"
	default:
		return "desktop"
	}
}

func validEventName(name string) bool {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == '-', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}

// cleanText reports whether s is valid UTF-8 with no control characters,
// i.e. safe to store in a Postgres TEXT column and to render.
func cleanText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
