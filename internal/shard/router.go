package shard

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	"github.com/jayant/nebuladb/internal/storage"
)

// Router implements storage.KV by hashing each key onto a shard.
type Router struct {
	mu     sync.RWMutex
	ring   *Ring
	stores map[ID]storage.KV
}

// NewRouter requires every ring id to have a store.
func NewRouter(ring *Ring, stores map[ID]storage.KV) (*Router, error) {
	if ring == nil {
		return nil, fmt.Errorf("shard: nil ring")
	}
	for _, id := range ring.IDs() {
		if stores[id] == nil {
			return nil, fmt.Errorf("shard: missing store %q", id)
		}
	}
	cp := make(map[ID]storage.KV, len(stores))
	for id, kv := range stores {
		cp[id] = kv
	}
	return &Router{ring: ring, stores: cp}, nil
}

// Owner is the shard id for key under the current ring.
func (r *Router) Owner(key []byte) ID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ring.Lookup(key)
}

func (r *Router) store(key []byte) storage.KV {
	return r.stores[r.ring.Lookup(key)]
}

// Get implements storage.KV.
func (r *Router) Get(key []byte) ([]byte, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.store(key).Get(key)
}

// Set implements storage.KV.
func (r *Router) Set(key, value []byte) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.store(key).Set(key, value)
}

// Delete implements storage.KV.
func (r *Router) Delete(key []byte) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.store(key).Delete(key)
}

type kvPair struct {
	k, v []byte
}

// ScanPrefix visits matching keys from every shard, sorted by key.
func (r *Router) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) {
	r.mu.RLock()
	stores := make([]storage.KV, 0, len(r.stores))
	for _, kv := range r.stores {
		stores = append(stores, kv)
	}
	r.mu.RUnlock()

	var pairs []kvPair
	for _, kv := range stores {
		kv.ScanPrefix(prefix, func(k, v []byte) bool {
			pairs = append(pairs, kvPair{append([]byte(nil), k...), append([]byte(nil), v...)})
			return true
		})
	}
	sort.Slice(pairs, func(i, j int) bool { return bytes.Compare(pairs[i].k, pairs[j].k) < 0 })
	for _, p := range pairs {
		if !fn(p.k, p.v) {
			return
		}
	}
}
