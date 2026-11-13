package sessions

import (
	"testing"
	"time"
)

// fake gives a deterministic clock and sequential IDs.
func fake(start time.Time) (*Tracker, *time.Time) {
	now := start
	next := int64(0)
	tr := New()
	tr.Now = func() time.Time { return now }
	tr.NewID = func() int64 { next++; return next }
	return tr, &now
}

var (
	visitorA = []byte("aaaaaaaaaaaaaaaa")
	visitorB = []byte("bbbbbbbbbbbbbbbb")
	// Same person after the midnight salt rotation: a different visitor ID.
	visitorARotated = []byte("AAAAAAAAAAAAAAAA")
)

func TestAssign(t *testing.T) {
	base := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	type ev struct {
		site    int64
		visitor []byte
		offset  time.Duration
	}

	cases := []struct {
		name   string
		start  time.Time
		events []ev
		want   []int64 // session ID per event, IDs are minted 1, 2, 3...
	}{
		{
			name:   "single event starts a session",
			events: []ev{{1, visitorA, 0}},
			want:   []int64{1},
		},
		{
			name: "events inside the window share a session",
			events: []ev{
				{1, visitorA, 0},
				{1, visitorA, 10 * time.Minute},
				{1, visitorA, 39 * time.Minute}, // 29m after the previous: still inside
			},
			want: []int64{1, 1, 1},
		},
		{
			name: "window is measured from the last event, not the first",
			events: []ev{
				{1, visitorA, 0},
				{1, visitorA, 25 * time.Minute},
				{1, visitorA, 50 * time.Minute},
				{1, visitorA, 75 * time.Minute},
			},
			want: []int64{1, 1, 1, 1},
		},
		{
			name: "exactly 30 minutes of inactivity starts a new session",
			events: []ev{
				{1, visitorA, 0},
				{1, visitorA, 30 * time.Minute},
			},
			want: []int64{1, 2},
		},
		{
			name: "29m59s keeps the session",
			events: []ev{
				{1, visitorA, 0},
				{1, visitorA, 29*time.Minute + 59*time.Second},
			},
			want: []int64{1, 1},
		},
		{
			name: "visitors are independent",
			events: []ev{
				{1, visitorA, 0},
				{1, visitorB, time.Minute},
				{1, visitorA, 2 * time.Minute},
			},
			want: []int64{1, 2, 1},
		},
		{
			name: "same visitor hash on two sites is two sessions",
			events: []ev{
				{1, visitorA, 0},
				{2, visitorA, time.Minute},
			},
			want: []int64{1, 2},
		},
		{
			name:  "midnight UTC splits a session even inside the window",
			start: time.Date(2026, 11, 13, 23, 50, 0, 0, time.UTC),
			events: []ev{
				{1, visitorA, 0},
				{1, visitorA, 15 * time.Minute}, // 00:05 next day
			},
			want: []int64{1, 2},
		},
		{
			name:  "midnight salt rotation changes the visitor ID",
			start: time.Date(2026, 11, 13, 23, 55, 0, 0, time.UTC),
			events: []ev{
				{1, visitorA, 0},
				{1, visitorARotated, 10 * time.Minute},
			},
			want: []int64{1, 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := tc.start
			if start.IsZero() {
				start = base
			}
			tr, _ := fake(start)
			for i, e := range tc.events {
				got := tr.Assign(e.site, e.visitor, start.Add(e.offset))
				if got != tc.want[i] {
					t.Fatalf("event %d: session = %d, want %d", i, got, tc.want[i])
				}
			}
		})
	}
}

func TestEvict(t *testing.T) {
	base := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	tr, now := fake(base)

	tr.Assign(1, visitorA, base)
	tr.Assign(1, visitorB, base.Add(20*time.Minute))

	*now = base.Add(29 * time.Minute)
	if n := tr.Evict(); n != 0 {
		t.Fatalf("evicted %d at 29m, want 0", n)
	}

	*now = base.Add(31 * time.Minute)
	if n := tr.Evict(); n != 1 {
		t.Fatalf("evicted %d at 31m, want 1 (visitor A)", n)
	}
	if tr.Len() != 1 {
		t.Fatalf("len = %d, want 1", tr.Len())
	}

	// B is still live, so its next event continues the same session.
	if got := tr.Assign(1, visitorB, base.Add(40*time.Minute)); got != 2 {
		t.Fatalf("visitor B session = %d, want 2", got)
	}

	*now = base.Add(71 * time.Minute)
	if n := tr.Evict(); n != 1 || tr.Len() != 0 {
		t.Fatalf("evicted %d, len %d; want 1, 0", n, tr.Len())
	}
}

func TestRandomIDIsPositive(t *testing.T) {
	for range 1000 {
		if id := randomID(); id <= 0 {
			t.Fatalf("randomID() = %d", id)
		}
	}
}
