// Package index implements LSM secondary indexes and order-preserving keys.
package index

import "bytes"

const (
	tagPrimary = 'p'
	tagIndex   = 'i'
	sep        = 0x00
)

var catalogKey = []byte("x\x00catalog")

// PrimaryKey is the LSM key for a row.
func PrimaryKey(pk []byte) []byte {
	out := make([]byte, 1+len(pk))
	out[0] = tagPrimary
	copy(out[1:], pk)
	return out
}

func parsePrimary(key []byte) (pk []byte, ok bool) {
	if len(key) < 2 || key[0] != tagPrimary {
		return nil, false
	}
	return key[1:], true
}

// encodeComparable is order-preserving: 0x00 is escaped so 0x00 0x01 is terminator.
func encodeComparable(b []byte) []byte {
	out := make([]byte, 0, len(b)+2)
	for _, c := range b {
		if c == 0 {
			out = append(out, 0, 0xff)
			continue
		}
		out = append(out, c)
	}
	return append(out, 0, 0x01)
}

func decodeComparable(b []byte) ([]byte, []byte, error) {
	var out []byte
	i := 0
	for i < len(b) {
		if b[i] == 0 {
			if i+1 >= len(b) {
				return nil, nil, errKey("short comparable")
			}
			if b[i+1] == 0x01 {
				return out, b[i+2:], nil
			}
			if b[i+1] == 0xff {
				out = append(out, 0)
				i += 2
				continue
			}
			return nil, nil, errKey("bad comparable escape")
		}
		out = append(out, b[i])
		i++
	}
	return nil, nil, errKey("unterminated comparable")
}

// SecondaryKey maps (index, sk, pk) → LSM key.
func SecondaryKey(idx, sk, pk []byte) []byte {
	enc := encodeComparable(sk)
	out := make([]byte, 0, 2+len(idx)+len(enc)+len(pk))
	out = append(out, tagIndex)
	out = append(out, idx...)
	out = append(out, sep)
	out = append(out, enc...)
	out = append(out, pk...)
	return out
}

// SecondaryPrefixAll is every key in one index.
func SecondaryPrefixAll(idx []byte) []byte {
	out := make([]byte, 0, 2+len(idx))
	out = append(out, tagIndex)
	out = append(out, idx...)
	out = append(out, sep)
	return out
}

// SecondaryPrefixSK is every pk for one secondary value.
func SecondaryPrefixSK(idx, sk []byte) []byte {
	enc := encodeComparable(sk)
	out := make([]byte, 0, 2+len(idx)+len(enc))
	out = append(out, tagIndex)
	out = append(out, idx...)
	out = append(out, sep)
	out = append(out, enc...)
	return out
}

func parseSecondary(key, idx []byte) (sk, pk []byte, ok bool) {
	pref := SecondaryPrefixAll(idx)
	if !bytes.HasPrefix(key, pref) {
		return nil, nil, false
	}
	rest := key[len(pref):]
	sk, rest, err := decodeComparable(rest)
	if err != nil {
		return nil, nil, false
	}
	return sk, rest, true
}

type keyError string

func (e keyError) Error() string { return string(e) }

func errKey(s string) error { return keyError("index: " + s) }
