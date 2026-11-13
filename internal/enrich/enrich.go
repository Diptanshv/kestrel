package enrich

import (
	"context"
	"time"
)

// Fields are the enriched values events_raw stores. Note what is absent: the
// IP and the user agent string went in and did not come out.
type Fields struct {
	VisitorID []byte
	Country   string
	Browser   string
	OS        string
	Device    string
	Bot       bool
}

type Enricher struct {
	Salts *SaltStore
	Geo   *Geo
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

	return Fields{
		VisitorID: VisitorID(salt, siteID, ip, userAgent),
		Country:   e.Geo.Country(ip),
		Browser:   ua.Browser,
		OS:        ua.OS,
		Device:    ua.Device,
	}, nil
}
