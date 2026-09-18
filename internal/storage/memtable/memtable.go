// Package memtable is an in-memory ordered skip list with tombstones.
package memtable

import (
	"bytes"
	"math/rand"
	"sync"
)

const (
	maxHeight = 16
	pNum      = 1
	pDen      = 4
)

type node struct {
	key     []byte
	value   []byte
	deleted bool
	height  int
	next    []*node
}

// Table is an ordered MemTable. All operations take t.mu; the skip list is
// the LSM-ready structure (sorted flush), not a lock-free experiment.
type Table struct {
	mu    sync.RWMutex
	head  *node
	rnd   *rand.Rand
	size  int64
	count int // live keys
}

// New returns an empty MemTable.
func New() *Table {
	return &Table{
		head: &node{height: maxHeight, next: make([]*node, maxHeight)},
		rnd:  rand.New(rand.NewSource(1)),
	}
}

func (t *Table) randomHeight() int {
	h := 1
	for h < maxHeight && t.rnd.Intn(pDen) < pNum {
		h++
	}
	return h
}

func (t *Table) find(key []byte, preds []*node) *node {
	x := t.head
	for i := maxHeight - 1; i >= 0; i-- {
		for x.next[i] != nil && bytes.Compare(x.next[i].key, key) < 0 {
			x = x.next[i]
		}
		if preds != nil {
			preds[i] = x
		}
	}
	n := x.next[0]
	if n != nil && bytes.Equal(n.key, key) {
		return n
	}
	return nil
}

func (t *Table) insertLocked(key, value []byte, deleted bool) {
	preds := make([]*node, maxHeight)
	if n := t.find(key, preds); n != nil {
		if !n.deleted {
			t.size -= int64(len(n.value))
			t.count--
		}
		n.value = value
		n.deleted = deleted
		if !deleted {
			t.size += int64(len(value))
			t.count++
		}
		return
	}
	h := t.randomHeight()
	n := &node{
		key:     bytes.Clone(key),
		value:   value,
		deleted: deleted,
		height:  h,
		next:    make([]*node, h),
	}
	for i := 0; i < h; i++ {
		n.next[i] = preds[i].next[i]
		preds[i].next[i] = n
	}
	t.size += int64(len(n.key) + len(value))
	if !deleted {
		t.count++
	}
}

// Put inserts or updates a key. A previous tombstone is cleared.
func (t *Table) Put(key, value []byte) {
	if len(key) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.insertLocked(key, bytes.Clone(value), false)
}

// Delete records a tombstone so later LSM compaction can hide older versions.
func (t *Table) Delete(key []byte) {
	if len(key) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.insertLocked(key, nil, true)
}

// Result is a MemTable lookup outcome (needed so tombstones hide older SSTables).
type Result int

const (
	Miss Result = iota
	Found
	Deleted
)

// Lookup distinguishes missing keys from tombstones.
func (t *Table) Lookup(key []byte) (value []byte, st Result) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := t.find(key, nil)
	if n == nil {
		return nil, Miss
	}
	if n.deleted {
		return nil, Deleted
	}
	return bytes.Clone(n.value), Found
}

// Get returns a copy of the value. ok is false if missing or tombstoned.
func (t *Table) Get(key []byte) (value []byte, ok bool) {
	v, st := t.Lookup(key)
	return v, st == Found
}

// Len is the number of live (non-tombstone) keys.
func (t *Table) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count
}

// ApproxSize is keys+values bytes, not including skip-list pointers.
func (t *Table) ApproxSize() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// Scan walks live keys in order. Callback must copy slices if it retains them.
func (t *Table) Scan(fn func(key, value []byte) bool) {
	t.ScanAll(func(key, value []byte, deleted bool) bool {
		if deleted {
			return true
		}
		return fn(key, value)
	})
}

// ScanAll visits every node, including tombstones, in key order.
func (t *Table) ScanAll(fn func(key, value []byte, deleted bool) bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for n := t.head.next[0]; n != nil; n = n.next[0] {
		if !fn(n.key, n.value, n.deleted) {
			return
		}
	}
}
