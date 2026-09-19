package raft

import "sync"

// Gate drops Raft RPCs for chaos tests and nebulactl partitions.
type Gate struct {
	mu      sync.Mutex
	drop    map[ID]bool
	dropAll bool
	split   map[ID]bool
	splitOn bool
}

// NewGate returns an open gate.
func NewGate() *Gate {
	return &Gate{drop: map[ID]bool{}, split: map[ID]bool{}}
}

// Isolate drops all RPCs to/from id.
func (g *Gate) Isolate(id ID, on bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.drop[id] = on
	g.mu.Unlock()
}

// DropAll blocks every send from this process when on.
func (g *Gate) DropAll(on bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.dropAll = on
	g.mu.Unlock()
}

// Split drops RPCs whose endpoints sit on different sides of left.
func (g *Gate) Split(left []ID, on bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.split = map[ID]bool{}
	for _, id := range left {
		g.split[id] = true
	}
	g.splitOn = on
	g.mu.Unlock()
}

// Heal clears isolate, drop-all, and split.
func (g *Gate) Heal() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.drop = map[ID]bool{}
	g.dropAll = false
	g.split = map[ID]bool{}
	g.splitOn = false
	g.mu.Unlock()
}

// Blocked reports whether a send from -> to should fail.
func (g *Gate) Blocked(from, to ID) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.dropAll {
		return true
	}
	if g.drop[from] || g.drop[to] {
		return true
	}
	if g.splitOn && g.split[from] != g.split[to] {
		return true
	}
	return false
}
