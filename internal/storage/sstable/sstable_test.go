package sstable

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteGetIter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.sst")
	entries := []Entry{
		{Key: []byte("a"), Value: []byte("1")},
		{Key: []byte("b"), Value: []byte("2"), Tombstone: true},
		{Key: []byte("c"), Value: []byte("3")},
	}
	if err := Write(path, entries); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	v, tomb, ok, err := r.Get([]byte("a"))
	if err != nil || !ok || tomb || string(v) != "1" {
		t.Fatalf("a: v=%q tomb=%v ok=%v err=%v", v, tomb, ok, err)
	}
	_, tomb, ok, err = r.Get([]byte("b"))
	if err != nil || !ok || !tomb {
		t.Fatalf("b tombstone: tomb=%v ok=%v err=%v", tomb, ok, err)
	}
	_, _, ok, err = r.Get([]byte("zzz"))
	if err != nil || ok {
		t.Fatalf("missing: ok=%v err=%v", ok, err)
	}

	it := r.Iter()
	var keys []string
	for it.Next() {
		keys = append(keys, string(it.Entry().Key))
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	if len(keys) != 3 || keys[0] != "a" || keys[2] != "c" {
		t.Fatalf("iter %v", keys)
	}
}

func TestRejectUnsorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.sst")
	err := Write(path, []Entry{
		{Key: []byte("b"), Value: []byte("1")},
		{Key: []byte("a"), Value: []byte("2")},
	})
	if err == nil {
		t.Fatal("expected unsorted error")
	}
}

func TestFooterCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.sst")
	if err := Write(path, []Entry{{Key: []byte("k"), Value: []byte("v")}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-1] ^= 0xff
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("expected crc error")
	}
}

func TestBloomSkipsAbsentKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.sst")
	if err := Write(path, []Entry{{Key: []byte("present"), Value: []byte("1")}}); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, _, ok, err := r.Get([]byte("absent-key-not-in-table"))
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	checked, neg := r.BloomStats()
	if checked != 1 || neg != 1 {
		t.Fatalf("bloom stats checked=%d neg=%d", checked, neg)
	}
}
