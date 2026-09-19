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

// Attach adds a physical shard and rebuilds the ring. Call Rebalance to move keys.
func (r *Router) Attach(id ID, kv storage.KV) error {
	if kv == nil || id == "" {
		return fmt.Errorf("shard: invalid attach")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.stores[id]; ok {
		return fmt.Errorf("shard: %q already attached", id)
	}
	ids := append(r.ring.IDs(), id)
	ring, err := NewRing(ids, r.ring.VNodes())
	if err != nil {
		return err
	}
	r.stores[id] = kv
	r.ring = ring
	return nil
}

// Rebalance moves keys whose owner changed. Set on the new shard, then delete on the old.
func (r *Router) Rebalance() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	type move struct {
		key, val []byte
		from, to ID
	}
	var moves []move
	for from, kv := range r.stores {
		from := from
		kv.ScanPrefix(nil, func(k, v []byte) bool {
			to := r.ring.Lookup(k)
			if to != from {
				moves = append(moves, move{
					key:  append([]byte(nil), k...),
					val:  append([]byte(nil), v...),
					from: from,
					to:   to,
				})
			}
			return true
		})
	}
	for _, m := range moves {
		if err := r.stores[m.to].Set(m.key, m.val); err != nil {
			return err
		}
		if err := r.stores[m.from].Delete(m.key); err != nil {
			return err
		}
	}
	return nil
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
