// Package bloom implements a Bloom filter for SSTable key existence checks.
package bloom

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

// Filter is a classic Bloom filter (false positives allowed, no false negatives).
type Filter struct {
	k     int
	nbits uint32
	bits  []byte
}

// New allocates a filter sized for n keys at false-positive rate fp (e.g. 0.01).
func New(n int, fp float64) *Filter {
	if n < 1 {
		n = 1
	}
	if fp <= 0 || fp >= 1 {
		fp = 0.01
	}
	m := uint32(math.Ceil(-float64(n) * math.Log(fp) / (math.Ln2 * math.Ln2)))
	if m < 64 {
		m = 64
	}
	k := int(math.Round(float64(m) / float64(n) * math.Ln2))
	if k < 1 {
		k = 1
	}
	if k > 16 {
		k = 16
	}
	return &Filter{k: k, nbits: m, bits: make([]byte, (m+7)/8)}
}

// Add inserts key.
func (f *Filter) Add(key []byte) {
	h1, h2 := hashPair(key)
	for i := 0; i < f.k; i++ {
		bit := (h1 + uint64(i)*h2) % uint64(f.nbits)
		f.bits[bit/8] |= 1 << (bit % 8)
	}
}

// MayContain reports whether key might be present.
func (f *Filter) MayContain(key []byte) bool {
	h1, h2 := hashPair(key)
	for i := 0; i < f.k; i++ {
		bit := (h1 + uint64(i)*h2) % uint64(f.nbits)
		if f.bits[bit/8]&(1<<(bit%8)) == 0 {
			return false
		}
	}
	return true
}

// Encode serializes k, nbits, and the bit array.
func (f *Filter) Encode() []byte {
	buf := make([]byte, 1+4+len(f.bits))
	buf[0] = byte(f.k)
	binary.LittleEndian.PutUint32(buf[1:5], f.nbits)
	copy(buf[5:], f.bits)
	return buf
}

// Decode loads a filter from Encode().
func Decode(buf []byte) (*Filter, error) {
	if len(buf) < 5 {
		return nil, errTrunc("bloom header")
	}
	k := int(buf[0])
	nbits := binary.LittleEndian.Uint32(buf[1:5])
	need := int((nbits + 7) / 8)
	if k < 1 || nbits < 1 || len(buf) != 5+need {
		return nil, errTrunc("bloom payload")
	}
	bits := make([]byte, need)
	copy(bits, buf[5:])
	return &Filter{k: k, nbits: nbits, bits: bits}, nil
}

type bloomError string

func (e bloomError) Error() string { return string(e) }

func errTrunc(msg string) error { return bloomError("bloom: truncated " + msg) }

func hashPair(key []byte) (uint64, uint64) {
	h := fnv.New64a()
	_, _ = h.Write(key)
	a := h.Sum64()
	h.Reset()
	_, _ = h.Write(key)
	var extra [1]byte
	extra[0] = 0x5f
	_, _ = h.Write(extra[:])
	b := h.Sum64()
	if b == 0 {
		b = 0x9e3779b97f4a7c15
	}
	return a, b
}
