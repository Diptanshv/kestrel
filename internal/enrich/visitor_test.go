package enrich

import (
	"bytes"
	"testing"
)

var (
	saltA = bytes.Repeat([]byte{0xA1}, SaltLen)
	saltB = bytes.Repeat([]byte{0xB2}, SaltLen)
)

func TestVisitorIDIsStableWithinADay(t *testing.T) {
	a := VisitorID(saltA, 1, "203.0.113.5", "Mozilla/5.0")
	b := VisitorID(saltA, 1, "203.0.113.5", "Mozilla/5.0")
	if !bytes.Equal(a, b) {
		t.Fatal("same inputs produced different IDs")
	}
	if len(a) != VisitorIDLen {
		t.Fatalf("len = %d, want %d", len(a), VisitorIDLen)
	}
}

func TestVisitorIDChangesWithTheSalt(t *testing.T) {
	// This is the property that makes IDs uncomparable across days.
	a := VisitorID(saltA, 1, "203.0.113.5", "Mozilla/5.0")
	b := VisitorID(saltB, 1, "203.0.113.5", "Mozilla/5.0")
	if bytes.Equal(a, b) {
		t.Fatal("a new salt produced the same ID")
	}
}

func TestVisitorIDSeparatesItsFields(t *testing.T) {
	tests := []struct {
		name          string
		siteID        int64
		ip, userAgent string
	}{
		{"different site", 2, "203.0.113.5", "Mozilla/5.0"},
		{"different ip", 1, "203.0.113.6", "Mozilla/5.0"},
		{"different ua", 1, "203.0.113.5", "Mozilla/4.0"},
	}
	base := VisitorID(saltA, 1, "203.0.113.5", "Mozilla/5.0")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if bytes.Equal(base, VisitorID(saltA, tt.siteID, tt.ip, tt.userAgent)) {
				t.Error("distinct inputs collided")
			}
		})
	}

	// Field boundaries must not be ambiguous.
	x := VisitorID(saltA, 1, "1.2.3.4", "Ab")
	y := VisitorID(saltA, 1, "1.2.3.4A", "b")
	if bytes.Equal(x, y) {
		t.Error("adjacent fields concatenated into the same message")
	}
}
