package hll

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	mrand "math/rand/v2"
	"os"
	"testing"
)

// randomIDs is seeded so accuracy assertions are reproducible rather than
// failing on an unlucky draw once in a few hundred runs.
func randomIDs(n int) [][]byte {
	r := mrand.New(mrand.NewPCG(uint64(n), 0x6b657374))
	out := make([][]byte, n)
	for i := range out {
		b := make([]byte, 16)
		binary.BigEndian.PutUint64(b, r.Uint64())
		binary.BigEndian.PutUint64(b[8:], r.Uint64())
		out[i] = b
	}
	return out
}

func relErr(est uint64, exact int) float64 {
	return math.Abs(float64(est)-float64(exact)) / float64(exact)
}

func TestEmpty(t *testing.T) {
	if got := New(12).Estimate(); got != 0 {
		t.Fatalf("empty estimate = %d", got)
	}
}

func TestSmallCountsAreNearExact(t *testing.T) {
	s := New(12)
	ids := randomIDs(100)
	for _, id := range ids {
		s.Add(id)
		s.Add(id) // duplicates must not count
	}
	if got := s.Estimate(); got < 95 || got > 105 {
		t.Fatalf("estimate for 100 = %d", got)
	}
}

func TestAccuracyAcrossCardinalities(t *testing.T) {
	// p=12 has a standard error of about 1.04/sqrt(4096) = 1.6%. Allow 4
	// sigma so the test is not flaky.
	for _, n := range []int{1_000, 5_000, 20_000, 100_000} {
		s := New(12)
		for _, id := range randomIDs(n) {
			s.Add(id)
		}
		if e := relErr(s.Estimate(), n); e > 0.065 {
			t.Errorf("n=%d: estimate %d, error %.2f%%", n, s.Estimate(), 100*e)
		}
	}
}

func TestMergeEqualsUnion(t *testing.T) {
	ids := randomIDs(30_000)
	a, b, all := New(12), New(12), New(12)
	for i, id := range ids {
		all.Add(id)
		// Overlapping halves: the middle third goes into both.
		if i < 20_000 {
			a.Add(id)
		}
		if i >= 10_000 {
			b.Add(id)
		}
	}
	if err := a.Merge(b); err != nil {
		t.Fatal(err)
	}
	if a.Estimate() != all.Estimate() {
		t.Fatalf("merged %d != union %d", a.Estimate(), all.Estimate())
	}
	if err := a.Merge(New(10)); err == nil {
		t.Fatal("merging different precisions should fail")
	}
}

func TestRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 50, 400, 5_000, 200_000} {
		s := New(12)
		for _, id := range randomIDs(n) {
			s.Add(id)
		}
		b, err := s.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		got, err := Unmarshal(b)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if got.Estimate() != s.Estimate() {
			t.Fatalf("n=%d: round trip %d != %d", n, got.Estimate(), s.Estimate())
		}
		if len(b) > headerBytes+s.packedLen() {
			t.Fatalf("n=%d: %d bytes exceeds dense size", n, len(b))
		}
	}
}

func TestSparseIsSmall(t *testing.T) {
	s := New(12)
	for _, id := range randomIDs(10) {
		s.Add(id)
	}
	b, _ := s.MarshalBinary()
	if len(b) > 40 {
		t.Fatalf("10 visitors encode to %d bytes; sparse form should be tiny", len(b))
	}
}

func TestUnmarshalRejectsGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, {9, 12, 0}, {1, 30, 0}, {1, 12, 7}, {1, 12, 1, 0}, {1, 12, 0, 5}} {
		if _, err := Unmarshal(b); err == nil {
			t.Errorf("Unmarshal(%v) succeeded", b)
		}
	}
}

func FuzzUnmarshal(f *testing.F) {
	s := New(10)
	for _, id := range randomIDs(500) {
		s.Add(id)
	}
	good, _ := s.MarshalBinary()
	f.Add(good)
	f.Add([]byte{1, 12, 0, 1, 64})
	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := Unmarshal(b)
		if err != nil {
			return
		}
		_ = s.Estimate()
		if _, err := s.MarshalBinary(); err != nil {
			t.Fatal(err)
		}
	})
}

// TestAccuracyReport prints error versus cardinality and sketch size for
// several precisions. Output feeds docs/benchmarks.md and docs/decisions.md:
//
//	KESTREL_BENCH=1 go test ./internal/hll -run TestAccuracyReport -v
func TestAccuracyReport(t *testing.T) {
	if os.Getenv("KESTREL_BENCH") == "" {
		t.Skip("set KESTREL_BENCH=1 to run")
	}
	const trials = 20
	cards := []int{10, 100, 1_000, 5_000, 10_000, 50_000, 100_000, 1_000_000}
	for _, p := range []uint8{10, 12, 14} {
		t.Logf("p=%d (m=%d, dense %d bytes)", p, 1<<p, headerBytes+(int(1)<<p*6+7)/8)
		for _, n := range cards {
			var sumErr, maxErr, size float64
			for range trials {
				s := New(p)
				var seed [8]byte
				_, _ = rand.Read(seed[:])
				base := binary.BigEndian.Uint64(seed[:])
				for i := range n {
					s.AddHash(mix64(base + uint64(i)))
				}
				e := relErr(s.Estimate(), n)
				sumErr += e
				maxErr = math.Max(maxErr, e)
				b, _ := s.MarshalBinary()
				size += float64(len(b))
			}
			t.Logf("  n=%-9d mean err %5.2f%%  max err %5.2f%%  size %s",
				n, 100*sumErr/trials, 100*maxErr, fmt.Sprintf("%.0f B", size/trials))
		}
	}
}

func BenchmarkAdd(b *testing.B) {
	s := New(12)
	ids := randomIDs(4096)
	i := 0
	for b.Loop() {
		s.Add(ids[i&4095])
		i++
	}
}

func BenchmarkMerge24Hourly(b *testing.B) {
	hours := make([]*Sketch, 24)
	for i := range hours {
		hours[i] = New(12)
		for _, id := range randomIDs(2_000) {
			hours[i].Add(id)
		}
	}
	for b.Loop() {
		day := New(12)
		for _, h := range hours {
			_ = day.Merge(h)
		}
		_ = day.Estimate()
	}
}
