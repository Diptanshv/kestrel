package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/Diptanshv/kestrel/internal/store"
)

const (
	siteCacheTTL         = time.Minute
	siteCacheNegativeTTL = 10 * time.Second
	siteCacheMaxEntries  = 10_000
)

// siteCache keeps the domain -> site lookup off the database for the ingest
// path. Unknown domains are cached briefly too, so a flood of beacons for an
// unregistered domain costs one query per 10 seconds rather than one each.
// Site mutations in this process invalidate their entries directly; the TTL
// bounds staleness for anything else.
type siteCache struct {
	q *store.Queries

	mu      sync.RWMutex
	entries map[string]siteCacheEntry
}

type siteCacheEntry struct {
	site    store.Site
	found   bool
	expires time.Time
}

func newSiteCache(q *store.Queries) *siteCache {
	return &siteCache{q: q, entries: make(map[string]siteCacheEntry)}
}

// get returns sql.ErrNoRows for unknown domains, like the query it fronts.
func (c *siteCache) get(ctx context.Context, domain string) (store.Site, error) {
	now := time.Now()
	c.mu.RLock()
	e, ok := c.entries[domain]
	c.mu.RUnlock()
	if ok && now.Before(e.expires) {
		if !e.found {
			return store.Site{}, sql.ErrNoRows
		}
		return e.site, nil
	}

	site, err := c.q.GetSiteByDomain(ctx, domain)
	switch {
	case err == nil:
		c.put(domain, siteCacheEntry{site: site, found: true, expires: now.Add(siteCacheTTL)})
	case errors.Is(err, sql.ErrNoRows):
		c.put(domain, siteCacheEntry{expires: now.Add(siteCacheNegativeTTL)})
	}
	return site, err
}

func (c *siteCache) put(domain string, e siteCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= siteCacheMaxEntries {
		// Crude but bounded: random domains cannot grow the map without limit.
		clear(c.entries)
	}
	c.entries[domain] = e
}

func (c *siteCache) invalidate(domains ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range domains {
		delete(c.entries, d)
	}
}
