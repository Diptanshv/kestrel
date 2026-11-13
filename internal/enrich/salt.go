package enrich

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/Diptanshv/kestrel/internal/store"
)

const (
	SaltLen = 32
	// Two days, so yesterday's sessions can still be closed out before the
	// salt that made them readable is gone.
	saltRetentionDays = 2
)

// SaltStore hands out the current day's salt, creating it on first use and
// caching it. The database is touched once per day, not once per event.
type SaltStore struct {
	q *store.Queries

	mu   sync.Mutex
	day  time.Time
	salt []byte
}

func NewSaltStore(q *store.Queries) *SaltStore { return &SaltStore{q: q} }

// For returns the salt for the UTC day containing now.
func (s *SaltStore) For(ctx context.Context, now time.Time) ([]byte, error) {
	day := now.UTC().Truncate(24 * time.Hour)

	s.mu.Lock()
	if s.salt != nil && s.day.Equal(day) {
		salt := s.salt
		s.mu.Unlock()
		return salt, nil
	}
	s.mu.Unlock()

	// Generated before the insert so the upsert is a single round trip. If
	// another instance won the race, its salt is what comes back.
	fresh := make([]byte, SaltLen)
	if _, err := rand.Read(fresh); err != nil {
		return nil, err
	}

	salt, err := s.q.UpsertSalt(ctx, store.UpsertSaltParams{Day: day, Salt: fresh})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.day, s.salt = day, salt
	s.mu.Unlock()

	// Rotation is the natural moment to drop expired salts, so no scheduler is
	// needed. Best effort: failing to prune must not fail an event.
	_ = s.q.DeleteSaltsBefore(ctx, day.AddDate(0, 0, -saltRetentionDays))

	return salt, nil
}
