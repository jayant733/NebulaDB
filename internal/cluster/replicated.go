// Package cluster wires Raft to the LSM engine.
package cluster

import (
	"github.com/jayant/nebuladb/internal/raft"
	"github.com/jayant/nebuladb/internal/storage"
)

// Replicated is a KV that proposes mutations through Raft.
type Replicated struct {
	Node  *raft.Node
	Local *storage.Engine
}

// New returns a replicated KV. Apply is installed on the Raft node via Start.
func Apply(local *storage.Engine) raft.ApplyFunc {
	return func(e raft.Entry) {
		switch e.Command.Op {
		case raft.OpSet:
			_ = local.Set(e.Command.Key, e.Command.Value)
		case raft.OpDelete:
			_ = local.Delete(e.Command.Key)
		}
	}
}

// Get implements storage.KV (local, may lag on followers).
func (r *Replicated) Get(key []byte) ([]byte, bool, error) {
	return r.Local.Get(key)
}

// Set proposes a put on the leader.
func (r *Replicated) Set(key, value []byte) error {
	return r.Node.Propose(raft.Command{Op: raft.OpSet, Key: key, Value: value})
}

// Delete proposes a delete on the leader.
func (r *Replicated) Delete(key []byte) error {
	return r.Node.Propose(raft.Command{Op: raft.OpDelete, Key: key})
}

// ScanPrefix implements storage.KV.
func (r *Replicated) ScanPrefix(prefix []byte, fn func(key, value []byte) bool) {
	r.Local.ScanPrefix(prefix, fn)
}
