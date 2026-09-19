package cluster

import (
	"github.com/jayant/nebuladb/internal/raft"
)

// LeaderPick is a KV that sends mutations to the current Raft leader of one shard.
type LeaderPick struct {
	Members []*Replicated
}

func (p *LeaderPick) leader() *Replicated {
	for _, m := range p.Members {
		if m != nil && m.Node != nil && m.Node.IsLeader() {
			return m
		}
	}
	return nil
}

// Get reads from the leader (or the first replica if there is no leader yet).
func (p *LeaderPick) Get(key []byte) ([]byte, bool, error) {
	if l := p.leader(); l != nil {
		return l.Get(key)
	}
	if len(p.Members) == 0 {
		return nil, false, raft.ErrNotLeader
	}
	return p.Members[0].Get(key)
}

// Set proposes on the leader.
func (p *LeaderPick) Set(key, value []byte) error {
	l := p.leader()
	if l == nil {
		return raft.ErrNotLeader
	}
	return l.Set(key, value)
}

// Delete proposes on the leader.
func (p *LeaderPick) Delete(key []byte) error {
	l := p.leader()
	if l == nil {
		return raft.ErrNotLeader
	}
	return l.Delete(key)
}

// ScanPrefix scans the leader's engine.
func (p *LeaderPick) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) {
	l := p.leader()
	if l == nil && len(p.Members) > 0 {
		l = p.Members[0]
	}
	if l != nil {
		l.ScanPrefix(prefix, fn)
	}
}
