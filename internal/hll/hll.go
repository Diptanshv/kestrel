// Package hll is a HyperLogLog cardinality sketch, sized for storing one per
// rollup row. Small sketches stay sparse (a few bytes per distinct visitor);
// large ones switch to 6-bit packed registers. Estimation uses Ertl's
// improved estimator ("New cardinality estimation algorithms for HyperLogLog
// sketches", 2017), which needs no empirical bias tables and stays accurate
// across the small-to-large transition where classic HLL bends.
package hll

import (
	"encoding/binary"
	"errors"
	"math"
	"math/bits"
	"sort"
)

// DefaultPrecision is what rollups use; see docs/decisions.md for the
// measurements behind it.
const DefaultPrecision = 12

const (
	minPrecision = 4
	maxPrecision = 16

	version     = 1
	encSparse   = 0
	encDense    = 1
	headerBytes = 3
)

// Sketch estimates the number of distinct values added to it. The zero value
// is not usable; call New.
type Sketch struct {
	p      uint8
	sparse map[uint32]uint8 // register index -> value, while small
	dense  []uint8          // one byte per register once large
}

func New(p uint8) *Sketch {
	if p < minPrecision || p > maxPrecision {
		panic("hll: precision out of range")
	}
	return &Sketch{p: p, sparse: make(map[uint32]uint8)}
}

func (s *Sketch) m() uint32 { return 1 << s.p }

// Precision reports p; the sketch has 2^p registers.
func (s *Sketch) Precision() uint8 { return s.p }

// Add inserts an identifier. Visitor IDs are already HMAC output, so their
// first 8 bytes are uniformly random; the mixer below is a guard for any
// other input, not a requirement.
func (s *Sketch) Add(id []byte) {
	var buf [8]byte
	copy(buf[:], id)
	s.AddHash(mix64(binary.BigEndian.Uint64(buf[:])))
}

// AddHash inserts a pre-hashed 64-bit value.
func (s *Sketch) AddHash(h uint64) {
	idx := uint32(h >> (64 - s.p))
	w := h<<s.p | 1<<(s.p-1) // sentinel bit caps rho at 64-p+1
	rho := uint8(bits.LeadingZeros64(w)) + 1
	s.set(idx, rho)
}

func (s *Sketch) set(idx uint32, rho uint8) {
	if s.dense != nil {
		if rho > s.dense[idx] {
			s.dense[idx] = rho
		}
		return
	}
	if rho > s.sparse[idx] {
		s.sparse[idx] = rho
	}
	// A map entry costs far more than a byte; go dense well before the
	// sparse form would outgrow it.
	if uint32(len(s.sparse)) > s.m()/8 {
		s.toDense()
	}
}

func (s *Sketch) toDense() {
	s.dense = make([]uint8, s.m())
	for i, v := range s.sparse {
		s.dense[i] = v
	}
	s.sparse = nil
}

// Merge folds o into s. Both must share a precision. Merging is lossless:
// the union of two sketches equals the sketch of the union.
func (s *Sketch) Merge(o *Sketch) error {
	if o.p != s.p {
		return errors.New("hll: precision mismatch")
	}
	if o.dense != nil {
		for i, v := range o.dense {
			if v != 0 {
				s.set(uint32(i), v)
			}
		}
		return nil
	}
	for i, v := range o.sparse {
		s.set(i, v)
	}
	return nil
}

// Estimate returns the approximate number of distinct values added.
func (s *Sketch) Estimate() uint64 {
	m := float64(s.m())
	q := 64 - int(s.p)
	hist := make([]float64, q+2)
	if s.dense != nil {
		for _, v := range s.dense {
			hist[v]++
		}
	} else {
		hist[0] = m - float64(len(s.sparse))
		for _, v := range s.sparse {
			hist[v]++
		}
	}
	if hist[0] == m {
		return 0
	}

	z := m * tau(1-hist[q+1]/m)
	for k := q; k >= 1; k-- {
		z = 0.5 * (z + hist[k])
	}
	z += m * sigma(hist[0]/m)
	const alphaInf = 0.5 / math.Ln2
	return uint64(math.Round(alphaInf * m * m / z))
}

func sigma(x float64) float64 {
	if x == 1 {
		return math.Inf(1)
	}
	y, z := 1.0, x
	for {
		x *= x
		prev := z
		z += x * y
		y += y
		if z == prev {
			return z
		}
	}
}

func tau(x float64) float64 {
	if x == 0 || x == 1 {
		return 0
	}
	y, z := 1.0, 1-x
	for {
		x = math.Sqrt(x)
		prev := z
		y *= 0.5
		z -= (1 - x) * (1 - x) * y
		if z == prev {
			return z / 3
		}
	}
}

// MarshalBinary encodes the sketch. Layout: version, precision, encoding,
// then either sparse entries (uvarint count, then delta-encoded uvarints of
// index<<6|value in index order) or dense registers packed 6 bits each.
// Whichever form is smaller is written.
func (s *Sketch) MarshalBinary() ([]byte, error) {
	sparse := s.sparseEntries()
	if sparse != nil {
		buf := make([]byte, headerBytes, headerBytes+binary.MaxVarintLen32*(len(sparse)+1))
		buf[0], buf[1], buf[2] = version, s.p, encSparse
		buf = binary.AppendUvarint(buf, uint64(len(sparse)))
		var prev uint32
		for _, e := range sparse {
			buf = binary.AppendUvarint(buf, uint64(e-prev))
			prev = e
		}
		if len(buf) <= headerBytes+s.packedLen() {
			return buf, nil
		}
	}

	regs := s.dense
	if regs == nil {
		regs = make([]uint8, s.m())
		for i, v := range s.sparse {
			regs[i] = v
		}
	}
	buf := make([]byte, headerBytes+s.packedLen())
	buf[0], buf[1], buf[2] = version, s.p, encDense
	pack6(buf[headerBytes:], regs)
	return buf, nil
}

// sparseEntries lists non-zero registers as sorted index<<6|value words, or
// nil when the sketch has too many to be worth it.
func (s *Sketch) sparseEntries() []uint32 {
	var out []uint32
	if s.dense != nil {
		for i, v := range s.dense {
			if v != 0 {
				out = append(out, uint32(i)<<6|uint32(v))
			}
		}
	} else {
		out = make([]uint32, 0, len(s.sparse))
		for i, v := range s.sparse {
			out = append(out, i<<6|uint32(v))
		}
		sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	}
	return out
}

func (s *Sketch) packedLen() int { return (int(s.m())*6 + 7) / 8 }

// Unmarshal decodes a sketch written by MarshalBinary.
func Unmarshal(b []byte) (*Sketch, error) {
	if len(b) < headerBytes || b[0] != version {
		return nil, errors.New("hll: bad header")
	}
	p := b[1]
	if p < minPrecision || p > maxPrecision {
		return nil, errors.New("hll: bad precision")
	}
	s := New(p)
	maxRho := 64 - p + 1 // anything larger cannot come from AddHash
	body := b[headerBytes:]
	switch b[2] {
	case encSparse:
		n, k := binary.Uvarint(body)
		if k <= 0 || n > uint64(s.m()) {
			return nil, errors.New("hll: bad sparse count")
		}
		body = body[k:]
		var cur uint32
		for range n {
			d, k := binary.Uvarint(body)
			if k <= 0 {
				return nil, errors.New("hll: truncated sparse entry")
			}
			body = body[k:]
			cur += uint32(d)
			idx, v := cur>>6, uint8(cur&0x3f)
			if idx >= s.m() || v > maxRho {
				return nil, errors.New("hll: register out of range")
			}
			s.set(idx, v)
		}
	case encDense:
		if len(body) != s.packedLen() {
			return nil, errors.New("hll: bad dense length")
		}
		s.dense = make([]uint8, s.m())
		s.sparse = nil
		unpack6(s.dense, body)
		for _, v := range s.dense {
			if v > maxRho {
				return nil, errors.New("hll: register out of range")
			}
		}
	default:
		return nil, errors.New("hll: unknown encoding")
	}
	return s, nil
}

func pack6(dst []byte, regs []uint8) {
	var acc uint32
	var nbits uint
	j := 0
	for _, r := range regs {
		acc = acc<<6 | uint32(r&0x3f)
		nbits += 6
		for nbits >= 8 {
			nbits -= 8
			dst[j] = byte(acc >> nbits)
			j++
		}
	}
	if nbits > 0 {
		dst[j] = byte(acc << (8 - nbits))
	}
}

func unpack6(regs []uint8, src []byte) {
	var acc uint32
	var nbits uint
	i := 0
	for _, b := range src {
		acc = acc<<8 | uint32(b)
		nbits += 8
		for nbits >= 6 && i < len(regs) {
			nbits -= 6
			regs[i] = uint8(acc>>nbits) & 0x3f
			i++
		}
	}
}

// mix64 is the splitmix64 finaliser.
func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
