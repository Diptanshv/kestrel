package ratelimit

import (
	"strconv"
	"testing"
	"time"
)

func fakeClock() (*time.Time, func() time.Time) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	return &now, func() time.Time { return now }
}

func TestBurstThenRefill(t *testing.T) {
	now, clock := fakeClock()
	l := New(2, 5) // 2/s, burst 5
	l.Now = clock

	for i := range 5 {
		if !l.Allow("a") {
			t.Fatalf("request %d within burst refused", i)
		}
	}
	if l.Allow("a") {
		t.Fatal("6th request allowed past burst")
	}
	if !l.Allow("b") {
		t.Fatal("keys must be independent")
	}

	*now = now.Add(500 * time.Millisecond) // +1 token
	if !l.Allow("a") || l.Allow("a") {
		t.Fatal("expected exactly one token after 500ms at 2/s")
	}

	*now = now.Add(time.Hour) // refill caps at burst
	n := 0
	for l.Allow("a") {
		n++
	}
	if n != 5 {
		t.Fatalf("after long idle allowed %d, want burst 5", n)
	}
}

func TestSweepAndMaxKeys(t *testing.T) {
	now, clock := fakeClock()
	l := New(1, 2)
	l.Now = clock
	l.MaxKeys = 3

	for i := range 3 {
		l.Allow(strconv.Itoa(i))
	}
	if l.Allow("new") {
		t.Fatal("table full of fresh buckets should refuse a new key")
	}

	*now = now.Add(2 * time.Second) // all buckets refilled -> sweepable
	if !l.Allow("new") {
		t.Fatal("new key refused after old buckets became sweepable")
	}
	if l.Len() != 1 {
		t.Fatalf("len = %d, want 1 after sweep", l.Len())
	}
}

func TestIPKey(t *testing.T) {
	cases := map[string]string{
		"203.0.113.9":          "203.0.113.9",
		"::ffff:203.0.113.9":   "203.0.113.9",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::/64",
		"not-an-ip":            "not-an-ip",
	}
	for in, want := range cases {
		if got := IPKey(in); got != want {
			t.Errorf("IPKey(%q) = %q, want %q", in, got, want)
		}
	}
}
