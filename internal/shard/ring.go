// Package shard routes LSM keys onto a consistent hash ring of engines.
package shard

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
)

// ID names a physical shard.
type ID string

const defaultVNodes = 64

type vnode struct {
	h  uint64
	id ID
}

// Ring is a Ketama-style consistent hash ring.
type Ring struct {
	vnodes []vnode // sorted by h
	ids    []ID
}

// NewRing places vnodes virtual nodes per id (default 64).
func NewRing(ids []ID, vnodes int) (*Ring, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("shard: no shard ids")
	}
	if vnodes <= 0 {
		vnodes = defaultVNodes
	}
	seen := map[ID]struct{}{}
	clean := make([]ID, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("shard: empty id")
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("shard: duplicate id %q", id)
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	vs := make([]vnode, 0, len(clean)*vnodes)
	for _, id := range clean {
		for i := 0; i < vnodes; i++ {
			vs = append(vs, vnode{h: hashString(string(id) + "#" + strconv.Itoa(i)), id: id})
		}
	}
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].h == vs[j].h {
			return vs[i].id < vs[j].id
		}
		return vs[i].h < vs[j].h
	})
	return &Ring{vnodes: vs, ids: clean}, nil
}

// Lookup returns the shard that owns key.
func (r *Ring) Lookup(key []byte) ID {
	h := hashBytes(key)
	vs := r.vnodes
	i := sort.Search(len(vs), func(i int) bool { return vs[i].h >= h })
	if i == len(vs) {
		return vs[0].id
	}
	return vs[i].id
}

// IDs is the physical membership in construction order.
func (r *Ring) IDs() []ID {
	out := make([]ID, len(r.ids))
	copy(out, r.ids)
	return out
}

func hashBytes(b []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

func hashString(s string) uint64 {
	return hashBytes([]byte(s))
}

// NumericIDs returns "0" .. "n-1".
func NumericIDs(n int) []ID {
	ids := make([]ID, n)
	for i := 0; i < n; i++ {
		ids[i] = ID(strconv.Itoa(i))
	}
	return ids
}
