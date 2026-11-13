package rollup

import (
	"sort"
	"time"

	"github.com/Diptanshv/kestrel/internal/store"
)

// Other is the value that absorbs everything past the top-N cap.
const Other = "(other)"

// DimTotal is the per-bucket total row; its value is always "".
const DimTotal = "_total"

type key struct {
	site      int64
	bucket    time.Time
	dimension string
	value     string
}

// agg accumulates one rollup row. visitors is an exact set here; Phase 9
// swaps it for a sketch so daily rows can be merged from hourly ones.
type agg struct {
	pageviews int
	visits    int
	bounces   int
	duration  int64
	visitors  map[[16]byte]struct{}
}

func (a *agg) addVisitor(v []byte) {
	if a.visitors == nil {
		a.visitors = make(map[[16]byte]struct{})
	}
	var k [16]byte
	copy(k[:], v)
	a.visitors[k] = struct{}{}
}

func (a *agg) merge(b *agg) {
	a.pageviews += b.pageviews
	a.visits += b.visits
	a.bounces += b.bounces
	a.duration += b.duration
	for v := range b.visitors {
		if a.visitors == nil {
			a.visitors = make(map[[16]byte]struct{})
		}
		a.visitors[v] = struct{}{}
	}
}

type dimValue struct{ dimension, value string }

// pageDims lists the rows a pageview contributes to. Empty attributes (no
// referrer, unknown country) are left out, matching the raw breakdowns.
func pageDims(e store.ListEventsForRollupRow) []dimValue {
	out := []dimValue{{DimTotal, ""}, {"page", e.Pathname}}
	for _, d := range []dimValue{
		{"referrer", e.Referrer},
		{"country", e.Country},
		{"browser", e.Browser},
		{"os", e.Os},
		{"device", e.Device},
	} {
		if d.value != "" {
			out = append(out, d)
		}
	}
	return out
}

type session struct {
	first     store.ListEventsForRollupRow
	last      time.Time
	pageviews int
	customs   int
}

// aggregate builds rollup rows for events in [from, to), bucketed by
// bucketOf. events may start earlier than from: those are only used to tell
// whether a session began before the window, so it is not counted twice.
//
// Pageview-derived rows count pageviews and distinct visitors. Visits,
// bounces and duration are attributed to the bucket a session started in,
// under the attributes of its entry event (entry page, entry referrer...).
func aggregate(events []store.ListEventsForRollupRow, bucketOf func(time.Time) time.Time, from, to time.Time) map[key]*agg {
	rows := make(map[key]*agg)
	get := func(site int64, bucket time.Time, d dimValue) *agg {
		k := key{site, bucket, d.dimension, d.value}
		a, ok := rows[k]
		if !ok {
			a = &agg{}
			rows[k] = a
		}
		return a
	}

	type sessKey struct {
		site int64
		id   int64
	}
	sessions := make(map[sessKey]*session)

	for _, e := range events {
		if e.SessionID != 0 {
			sk := sessKey{e.SiteID, e.SessionID}
			s, ok := sessions[sk]
			if !ok {
				s = &session{first: e}
				sessions[sk] = s
			}
			s.last = e.Ts
			if e.Name == "pageview" {
				s.pageviews++
			} else {
				s.customs++
			}
		}

		if e.Ts.Before(from) || !e.Ts.Before(to) {
			continue
		}
		bucket := bucketOf(e.Ts)
		if e.Name == "pageview" {
			for _, d := range pageDims(e) {
				a := get(e.SiteID, bucket, d)
				a.pageviews++
				a.addVisitor(e.VisitorID)
			}
			continue
		}
		a := get(e.SiteID, bucket, dimValue{"event", e.Name})
		a.pageviews++
		a.addVisitor(e.VisitorID)
	}

	for _, s := range sessions {
		start := s.first.Ts
		if start.Before(from) || !start.Before(to) {
			continue
		}
		bounce := s.pageviews == 1 && s.customs == 0
		dur := int64(s.last.Sub(start) / time.Second)
		bucket := bucketOf(start)
		for _, d := range pageDims(s.first) {
			a := get(s.first.SiteID, bucket, d)
			a.visits++
			a.duration += dur
			if bounce {
				a.bounces++
			}
		}
	}
	return rows
}

// capValues keeps the topN values per (site, bucket, dimension) by
// pageviews and folds the rest into a single Other row. Totals are exempt.
// This bounds rollup size no matter how many distinct paths or referrers a
// site (or an attacker) produces.
func capValues(rows map[key]*agg, topN int) map[key]*agg {
	type group struct {
		site      int64
		bucket    time.Time
		dimension string
	}
	groups := make(map[group][]key)
	for k := range rows {
		if k.dimension == DimTotal {
			continue
		}
		g := group{k.site, k.bucket, k.dimension}
		groups[g] = append(groups[g], k)
	}

	for g, keys := range groups {
		if len(keys) <= topN {
			continue
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := rows[keys[i]], rows[keys[j]]
			if a.pageviews != b.pageviews {
				return a.pageviews > b.pageviews
			}
			return keys[i].value < keys[j].value
		})
		otherKey := key{g.site, g.bucket, g.dimension, Other}
		other, ok := rows[otherKey]
		if !ok {
			other = &agg{}
		}
		for _, k := range keys[topN:] {
			if k.value == Other {
				continue
			}
			other.merge(rows[k])
			delete(rows, k)
		}
		rows[otherKey] = other
	}
	return rows
}

// TruncHour and TruncDay bucket in loc. time.Truncate cannot be used: it
// works on absolute time, so in UTC+5:30 it would cut at :30 past the hour.
func TruncHour(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), 0, 0, 0, loc)
}

func TruncDay(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
}

// AsDate turns a local midnight into the UTC-midnight time.Time that a DATE
// column round-trips as, keeping the local calendar date.
func AsDate(localMidnight time.Time) time.Time {
	return time.Date(localMidnight.Year(), localMidnight.Month(), localMidnight.Day(), 0, 0, 0, 0, time.UTC)
}

// FromDate is the inverse of AsDate.
func FromDate(d time.Time, loc *time.Location) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
}
