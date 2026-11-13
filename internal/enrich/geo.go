package enrich

import (
	"net"

	"github.com/oschwald/maxminddb-golang"
)

// Geo resolves an IP to a two-letter country code. A zero Geo - which is what
// you get when no database is configured - resolves everything to "".
type Geo struct {
	db *maxminddb.Reader
}

// OpenGeo loads the database at path. An empty path is not an error: country
// lookup is simply disabled, which keeps local dev and CI free of a binary
// dependency.
func OpenGeo(path string) (*Geo, error) {
	if path == "" {
		return &Geo{}, nil
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &Geo{db: db}, nil
}

// Country returns an ISO 3166-1 alpha-2 code, or "" when unknown. The IP is
// read here and not retained.
func (g *Geo) Country(ip string) string {
	if g == nil || g.db == nil {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}

	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := g.db.Lookup(parsed, &rec); err != nil {
		return ""
	}
	// events_raw.country is CHAR(2); anything else is a database error waiting
	// to happen, so drop it rather than pad it.
	if len(rec.Country.ISOCode) != 2 {
		return ""
	}
	return rec.Country.ISOCode
}

func (g *Geo) Close() error {
	if g == nil || g.db == nil {
		return nil
	}
	return g.db.Close()
}
