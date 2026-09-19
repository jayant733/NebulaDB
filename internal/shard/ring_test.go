package shard

import (
	"strconv"
	"testing"
)

func TestLookupStable(t *testing.T) {
	r, err := NewRing(NumericIDs(3), 64)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("user:42")
	id := r.Lookup(key)
	for i := 0; i < 100; i++ {
		if r.Lookup(key) != id {
			t.Fatal("unstable lookup")
		}
	}
}

func TestLookupSpreads(t *testing.T) {
	r, err := NewRing(NumericIDs(3), 64)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[ID]int{}
	for i := 0; i < 3000; i++ {
		k := []byte("k" + strconv.Itoa(i))
		counts[r.Lookup(k)]++
	}
	if len(counts) != 3 {
		t.Fatalf("expected 3 shards, got %v", counts)
	}
	for id, n := range counts {
		if n < 500 {
			t.Fatalf("shard %s too small: %d", id, n)
		}
	}
}

func TestAddShardRemapsFraction(t *testing.T) {
	r3, err := NewRing(NumericIDs(3), 64)
	if err != nil {
		t.Fatal(err)
	}
	r4, err := NewRing(NumericIDs(4), 64)
	if err != nil {
		t.Fatal(err)
	}
	moved := 0
	const n = 4000
	for i := 0; i < n; i++ {
		k := []byte("m" + strconv.Itoa(i))
		if r3.Lookup(k) != r4.Lookup(k) {
			moved++
		}
	}
	frac := float64(moved) / n
	if frac < 0.05 || frac > 0.5 {
		t.Fatalf("moved fraction %v (want roughly 1/4)", frac)
	}
}

func TestNewRingRejectsEmpty(t *testing.T) {
	if _, err := NewRing(nil, 8); err == nil {
		t.Fatal("expected error")
	}
}
