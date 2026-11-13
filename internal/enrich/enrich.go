package enrich

import (
	"context"
	"time"

	"github.com/Diptanshv/kestrel/internal/sessions"
)

// Fields are the enriched values events_raw stores. Note what is absent: the
// IP and the user agent string went in and did not come out.
type Fields struct {
	VisitorID []byte
	SessionID int64
	Country   string
	Browser   string
	OS        string
	Device    string
	Bot       bool
}

type Enricher struct {
	Salts *SaltStore
	Geo   *Geo
	// Sessions assigns visits. Nil leaves SessionID at 0, which the stats
	// queries treat as "no session".
	Sessions *sessions.Tracker
}

func New(salts *SaltStore, geo *Geo) *Enricher {
	return &Enricher{Salts: salts, Geo: geo}
}

// Enrich resolves everything that needs the raw request. Callers must check
// Bot and drop the event rather than storing it.
func (e *Enricher) Enrich(ctx context.Context, siteID int64, ip, userAgent string, now time.Time) (Fields, error) {
	ua := ParseUA(userAgent)
	if ua.Bot {
		// No salt lookup, no geo lookup: a bot costs one UA parse.
		return Fields{Bot: true}, nil
	}

	salt, err := e.Salts.For(ctx, now)
	if err != nil {
		return Fields{}, err
	}

	visitor := VisitorID(salt, siteID, ip, userAgent)
	var session int64
	if e.Sessions != nil {
		session = e.Sessions.Assign(siteID, visitor, now)
	}

	return Fields{
		VisitorID: visitor,
		SessionID: session,
		Country:   e.Geo.Country(ip),
		Browser:   ua.Browser,
		OS:        ua.OS,
		Device:    ua.Device,
	}, nil
}
