// Package ratelimit is an in-memory token bucket keyed by string. Like the
// session tracker it assumes a single API instance; see docs/security.md.
package ratelimit

import (
	"context"
	"net/netip"
	"sync"
	"time"
)

// Limiter allows Rate events per second per key, with bursts up to Burst.
type Limiter struct {
	Rate  float64
	Burst float64
	// MaxKeys bounds memory. When a new key arrives and the table is full,
	// idle buckets are swept; if it is still full the request is refused
	// rather than letting a key flood grow the map without limit.
	MaxKeys int
	Now     func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func New(rate, burst float64) *Limiter {
	return &Limiter{
		Rate:    rate,
		Burst:   burst,
		MaxKeys: 100_000,
		Now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// Allow takes one token for key, reporting whether one was available.
func (l *Limiter) Allow(key string) bool {
	now := l.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.MaxKeys {
			l.sweepLocked(now)
			if len(l.buckets) >= l.MaxKeys {
				return false
			}
		}
		b = &bucket{tokens: l.Burst, last: now}
		l.buckets[key] = b
	}

	b.tokens += now.Sub(b.last).Seconds() * l.Rate
	if b.tokens > l.Burst {
		b.tokens = l.Burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Sweep drops buckets that have refilled completely; they hold no state a
// fresh bucket would not.
func (l *Limiter) Sweep() {
	now := l.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(now)
}

func (l *Limiter) sweepLocked(now time.Time) {
	full := time.Duration(l.Burst / l.Rate * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) >= full {
			delete(l.buckets, k)
		}
	}
}

// Len reports how many keys are tracked.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// RunSweeper sweeps every interval until ctx ends.
func (l *Limiter) RunSweeper(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.Sweep()
		}
	}
}

// IPKey turns an address into a limiter key. IPv6 clients are grouped by
// /64: one host typically controls a whole /64, so per-address buckets would
// let it rotate through billions of keys.
func IPKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is6() {
		p, _ := addr.Prefix(64)
		return p.String()
	}
	return addr.String()
}
