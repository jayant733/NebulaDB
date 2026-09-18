// Package index implements LSM secondary indexes and order-preserving keys.
package index

import "bytes"

const (
	tagPrimary = 'p'
	tagIndex   = 'i'
	sep        = 0x00
)

func catalogKey(table string) []byte {
	k := []byte("x\x00catalog\x00")
	return append(k, table...)
}

// PrimaryPrefix is all primary rows for a table.
func PrimaryPrefix(table string) []byte {
	out := make([]byte, 0, 2+len(table))
	out = append(out, tagPrimary)
	out = append(out, table...)
	out = append(out, sep)
	return out
}

// PrimaryKey is the LSM key for a row in table.
func PrimaryKey(table string, pk []byte) []byte {
	out := PrimaryPrefix(table)
	return append(out, pk...)
}

func parsePrimary(table string, key []byte) (pk []byte, ok bool) {
	pref := PrimaryPrefix(table)
	if !bytes.HasPrefix(key, pref) || len(key) <= len(pref) {
		return nil, false
	}
	return key[len(pref):], true
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

func indexPrefix(table, idx string) []byte {
	out := make([]byte, 0, 3+len(table)+len(idx))
	out = append(out, tagIndex)
	out = append(out, table...)
	out = append(out, sep)
	out = append(out, idx...)
	out = append(out, sep)
	return out
}

// SecondaryKey maps (table, index, sk, pk) → LSM key.
func SecondaryKey(table, idx string, sk, pk []byte) []byte {
	enc := encodeComparable(sk)
	out := indexPrefix(table, idx)
	out = append(out, enc...)
	out = append(out, pk...)
	return out
}

// SecondaryPrefixAll is every key in one index of a table.
func SecondaryPrefixAll(table, idx string) []byte {
	return indexPrefix(table, idx)
}

// SecondaryPrefixSK is every pk for one secondary value.
func SecondaryPrefixSK(table, idx string, sk []byte) []byte {
	enc := encodeComparable(sk)
	out := indexPrefix(table, idx)
	return append(out, enc...)
}

func parseSecondary(table, idx string, key []byte) (sk, pk []byte, ok bool) {
	pref := indexPrefix(table, idx)
	if !bytes.HasPrefix(key, pref) {
		return nil, nil, false
	}
	sk, rest, err := decodeComparable(key[len(pref):])
	if err != nil {
		return nil, nil, false
	}
	return sk, rest, true
}

type keyError string

func (e keyError) Error() string { return string(e) }

func errKey(s string) error { return keyError("index: " + s) }
