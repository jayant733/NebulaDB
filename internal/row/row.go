// Package row encodes tuples stored as primary-row values.
package row

import (
	"encoding/binary"
	"fmt"
)

// Encode writes n fields as little-endian length-prefixed bytes.
func Encode(fields [][]byte) []byte {
	n := len(fields)
	size := 4
	for _, f := range fields {
		size += 4 + len(f)
	}
	buf := make([]byte, size)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(n))
	p := 4
	for _, f := range fields {
		binary.LittleEndian.PutUint32(buf[p:p+4], uint32(len(f)))
		p += 4
		copy(buf[p:], f)
		p += len(f)
	}
	return buf
}

// Decode reads Encode output.
func Decode(buf []byte) ([][]byte, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("row: short header")
	}
	n := binary.LittleEndian.Uint32(buf[0:4])
	p := 4
	out := make([][]byte, 0, n)
	for i := uint32(0); i < n; i++ {
		if p+4 > len(buf) {
			return nil, fmt.Errorf("row: truncated field length")
		}
		ln := binary.LittleEndian.Uint32(buf[p : p+4])
		p += 4
		if p+int(ln) > len(buf) {
			return nil, fmt.Errorf("row: truncated field")
		}
		f := make([]byte, ln)
		copy(f, buf[p:p+int(ln)])
		out = append(out, f)
		p += int(ln)
	}
	if p != len(buf) {
		return nil, fmt.Errorf("row: trailing bytes")
	}
	return out, nil
}

// Field returns fields[i] or false if missing.
func Field(fields [][]byte, i int) ([]byte, bool) {
	if i < 0 || i >= len(fields) {
		return nil, false
	}
	return fields[i], true
}
