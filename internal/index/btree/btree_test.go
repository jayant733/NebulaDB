package btree

import (
	"fmt"
	"testing"
)

func TestPutGet(t *testing.T) {
	tr := New(4)
	tr.Put([]byte("b"), []byte("2"))
	tr.Put([]byte("a"), []byte("1"))
	tr.Put([]byte("c"), []byte("3"))
	v, ok := tr.Get([]byte("a"))
	if !ok || string(v) != "1" {
		t.Fatalf("%q %v", v, ok)
	}
	tr.Put([]byte("a"), []byte("x"))
	v, ok = tr.Get([]byte("a"))
	if !ok || string(v) != "x" {
		t.Fatal("update")
	}
	if _, ok := tr.Get([]byte("z")); ok {
		t.Fatal("missing")
	}
}

func TestSplitsAndRange(t *testing.T) {
	tr := New(4)
	const n = 40
	for i := n; i >= 1; i-- {
		k := []byte(fmt.Sprintf("%02d", i))
		tr.Put(k, k)
	}
	for i := 1; i <= n; i++ {
		k := []byte(fmt.Sprintf("%02d", i))
		v, ok := tr.Get(k)
		if !ok || string(v) != string(k) {
			t.Fatalf("get %s: %q %v", k, v, ok)
		}
	}
	var got []string
	tr.Range([]byte("10"), []byte("15"), true, func(k, v []byte) bool {
		got = append(got, string(k))
		return true
	})
	if len(got) != 6 || got[0] != "10" || got[5] != "15" {
		t.Fatalf("range %v", got)
	}
}

func TestRangeUnbounded(t *testing.T) {
	tr := New(3)
	tr.Put([]byte("m"), []byte("1"))
	tr.Put([]byte("a"), []byte("1"))
	tr.Put([]byte("z"), []byte("1"))
	var n int
	tr.Range(nil, nil, true, func(k, v []byte) bool {
		n++
		return true
	})
	if n != 3 {
		t.Fatalf("n=%d", n)
	}
}
