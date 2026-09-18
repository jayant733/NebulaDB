package index

import (
	"bytes"
	"testing"
)

func TestSecondaryOrder(t *testing.T) {
	idx := []byte("age")
	k1 := SecondaryKey(idx, []byte("10"), []byte("a"))
	k2 := SecondaryKey(idx, []byte("10"), []byte("b"))
	k3 := SecondaryKey(idx, []byte("11"), []byte("a"))
	if bytes.Compare(k1, k2) >= 0 || bytes.Compare(k2, k3) >= 0 {
		t.Fatal("expected k1 < k2 < k3")
	}
	pref := SecondaryPrefixSK(idx, []byte("10"))
	if !bytes.HasPrefix(k1, pref) || !bytes.HasPrefix(k2, pref) || bytes.HasPrefix(k3, pref) {
		t.Fatal("prefix")
	}
	sk, pk, ok := parseSecondary(k1, idx)
	if !ok || string(sk) != "10" || string(pk) != "a" {
		t.Fatalf("%s %s %v", sk, pk, ok)
	}
}

func TestComparableNUL(t *testing.T) {
	a := encodeComparable([]byte{0})
	b := encodeComparable([]byte{0, 1})
	if bytes.Compare(a, b) >= 0 {
		t.Fatal("0 should sort before 0,1")
	}
	got, rest, err := decodeComparable(append(a, 'x'))
	if err != nil || len(got) != 1 || got[0] != 0 || string(rest) != "x" {
		t.Fatalf("%q %q %v", got, rest, err)
	}
}
