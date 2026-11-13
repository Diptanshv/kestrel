// Package sessions groups a visitor's events into visits. State is in memory,
// which pins the service to one instance; see docs/decisions.md.
package sessions

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

// DefaultTimeout is the inactivity gap that ends a visit.
const DefaultTimeout = 30 * time.Minute

type key struct {
	siteID  int64
	visitor [16]byte
}

type state struct {
	id       int64
	lastSeen time.Time
}

// Tracker assigns session IDs. A visitor's next event joins their current
// session unless it arrives Timeout or more after the previous one, or falls on
// a different UTC day. The day rule makes explicit what salt rotation already
// implies: visitor IDs change at midnight UTC, so sessions cannot span it.
type Tracker struct {
	Timeout time.Duration
	// Now is the clock used for eviction. Tests replace it.
	Now func() time.Time
	// NewID mints session IDs. Random 63-bit values, so IDs stay unique
	// across restarts without coordination; tests replace it for readability.
	NewID func() int64

	mu     sync.Mutex
	active map[key]*state
}

func New() *Tracker {
	return &Tracker{
		Timeout: DefaultTimeout,
		Now:     time.Now,
		NewID:   randomID,
		active:  make(map[key]*state),
	}
}

// Assign returns the session for an event at time at. Events are expected in
// roughly increasing time order per visitor, which holds because at is the
// server's receive time.
func (t *Tracker) Assign(siteID int64, visitorID []byte, at time.Time) int64 {
	k := key{siteID: siteID}
	copy(k.visitor[:], visitorID)

	t.mu.Lock()
	defer t.mu.Unlock()

	if s, ok := t.active[k]; ok && t.continues(s.lastSeen, at) {
		if at.After(s.lastSeen) {
			s.lastSeen = at
		}
		return s.id
	}
	id := t.NewID()
	t.active[k] = &state{id: id, lastSeen: at}
	return id
}

func (t *Tracker) continues(lastSeen, at time.Time) bool {
	if at.Sub(lastSeen) >= t.Timeout {
		return false
	}
	return sameUTCDay(lastSeen, at)
}

// Evict drops sessions idle for at least Timeout and returns how many went.
// It only bounds memory: Assign already ignores expired entries.
func (t *Tracker) Evict() int {
	now := t.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for k, s := range t.active {
		if now.Sub(s.lastSeen) >= t.Timeout {
			delete(t.active, k)
			n++
		}
	}
	return n
}

// Len reports the number of tracked sessions.
func (t *Tracker) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.active)
}

// Run evicts every interval until ctx is done.
func (t *Tracker) Run(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.Evict()
		}
	}
}

func sameUTCDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func randomID() int64 {
	var b [8]byte
	for {
		_, _ = rand.Read(b[:])
		// Mask the sign bit; 0 is reserved for "no session" on legacy rows.
		if id := int64(binary.BigEndian.Uint64(b[:]) &^ (1 << 63)); id != 0 {
			return id
		}
	}
}
