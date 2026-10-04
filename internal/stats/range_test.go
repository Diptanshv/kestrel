package stats

import (
	"testing"
	"time"
)

var now = time.Date(2026, 10, 4, 12, 30, 0, 0, time.UTC)

func TestParseRange(t *testing.T) {
	t.Run("defaults to last 7 days", func(t *testing.T) {
		r, err := ParseRange("", "", now)
		if err != nil {
			t.Fatalf("ParseRange() error = %v", err)
		}
		if got := r.To.Sub(r.From); got != 7*24*time.Hour {
			t.Errorf("width = %v, want %v", got, 7*24*time.Hour)
		}
	})

	t.Run("bare date is midnight UTC", func(t *testing.T) {
		r, err := ParseRange("2026-10-01", "2026-10-02", now)
		if err != nil {
			t.Fatalf("ParseRange() error = %v", err)
		}
		want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		if !r.From.Equal(want) {
			t.Errorf("From = %v, want %v", r.From, want)
		}
	})

	for _, tc := range []struct{ name, from, to string }{
		{"to before from", "2026-10-02", "2026-10-01"},
		{"zero width", "2026-10-01", "2026-10-01"},
		{"unparseable", "last tuesday", ""},
		{"range too wide", "2000-01-01", "2026-01-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseRange(tc.from, tc.to, now); err == nil {
				t.Error("ParseRange() error = nil, want an error")
			}
		})
	}
}

func TestParseInterval(t *testing.T) {
	short := Range{From: now.Add(-24 * time.Hour), To: now}
	long := Range{From: now.AddDate(0, 0, -30), To: now}

	tests := []struct {
		in   string
		r    Range
		want Interval
	}{
		{"", short, IntervalHour},
		{"", long, IntervalDay},
		{"hour", long, IntervalHour},
		{"day", short, IntervalDay},
	}
	for _, tt := range tests {
		got, err := ParseInterval(tt.in, tt.r)
		if err != nil {
			t.Fatalf("ParseInterval(%q) error = %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParseInterval(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	if _, err := ParseInterval("week", short); err == nil {
		t.Error("ParseInterval(\"week\") error = nil, want an error")
	}
}
