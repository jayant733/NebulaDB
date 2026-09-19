package cluster

import (
	"errors"
	"testing"
	"time"

	"github.com/jayant/nebuladb/internal/raft"
	"github.com/jayant/nebuladb/internal/storage"
)

func TestReplicatedSetOnLeader(t *testing.T) {
	net := raft.NewMemory()
	ids := []raft.ID{"a", "b", "c"}
	engines := make([]*storage.Engine, 3)
	nodes := make([]*raft.Node, 3)
	kvs := make([]*Replicated, 3)
	for i, id := range ids {
		eng, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncNone})
		if err != nil {
			t.Fatal(err)
		}
		defer eng.Close()
		engines[i] = eng
		n, err := raft.Start(raft.Config{
			ID:          id,
			Peers:       ids,
			Dir:         t.TempDir(),
			Transport:   net,
			Apply:       Apply(eng),
			ElectionMin: 50 * time.Millisecond,
			ElectionMax: 120 * time.Millisecond,
			Heartbeat:   20 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		net.Register(n)
		nodes[i] = n
		kvs[i] = &Replicated{Node: n, Local: eng}
		defer n.Stop()
	}
	lead := waitLeader(t, nodes)
	var kv *Replicated
	for i, n := range nodes {
		if n == lead {
			kv = kvs[i]
		}
	}
	if err := kv.Set([]byte("hello"), []byte("world")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ok := true
		for _, eng := range engines {
			v, found, err := eng.Get([]byte("hello"))
			if err != nil || !found || string(v) != "world" {
				ok = false
			}
		}
		if ok {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	for _, n := range nodes {
		if n == lead {
			continue
		}
		if err := (&Replicated{Node: n, Local: engines[0]}).Set([]byte("x"), []byte("y")); !errors.Is(err, raft.ErrNotLeader) {
			t.Fatalf("follower write: %v", err)
		}
	}
}

func waitLeader(t *testing.T, nodes []*raft.Node) *raft.Node {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var ls []*raft.Node
		for _, n := range nodes {
			if n.IsLeader() {
				ls = append(ls, n)
			}
		}
		if len(ls) == 1 {
			return ls[0]
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("expected one leader")
	return nil
}
