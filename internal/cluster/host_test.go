package cluster

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jayant/nebuladb/internal/raft"
	"github.com/jayant/nebuladb/internal/shard"
	"github.com/jayant/nebuladb/internal/storage"
)

func TestReplicatedShardsElectAndWrite(t *testing.T) {
	hosts, nets := startHosts(t, 3, 2)
	defer closeHosts(hosts)

	lead0 := waitShardLeader(t, hosts, 0)
	lead1 := waitShardLeader(t, hosts, 1)
	if lead0 == nil || lead1 == nil {
		t.Fatal("missing leaders")
	}

	rt, err := LeaderRouter(hosts)
	if err != nil {
		t.Fatal(err)
	}
	k0 := keyOn(t, rt, "0")
	k1 := keyOn(t, rt, "1")
	if err := rt.Set(k0, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := rt.Set(k1, []byte("b")); err != nil {
		t.Fatal(err)
	}
	v, ok, err := rt.Get(k0)
	if err != nil || !ok || string(v) != "a" {
		t.Fatalf("k0 %q %v %v", v, ok, err)
	}
	v, ok, err = rt.Get(k1)
	if err != nil || !ok || string(v) != "b" {
		t.Fatalf("k1 %q %v %v", v, ok, err)
	}

	// Follower of shard 0 rejects writes.
	var follower *Replicated
	for _, h := range hosts {
		if !h.Replicas[0].Node.IsLeader() {
			follower = h.Replicas[0].KV
			break
		}
	}
	if follower == nil {
		t.Fatal("no follower")
	}
	if err := follower.Set([]byte("x"), []byte("y")); !errors.Is(err, raft.ErrNotLeader) {
		t.Fatalf("follower: %v", err)
	}

	// Kill shard-0 leader; shard 1 still accepts writes.
	nets[0].(*raft.Memory).Isolate(lead0.ID(), true)
	lead0.Stop()
	if err := rt.Set(k1, []byte("b2")); err != nil {
		t.Fatal(err)
	}
	waitShardLeader(t, hosts, 0)
	if err := rt.Set(k0, []byte("a2")); err != nil {
		t.Fatal(err)
	}
	v, ok, err = rt.Get(k0)
	if err != nil || !ok || string(v) != "a2" {
		t.Fatalf("after failover %q %v %v", v, ok, err)
	}
}

func startHosts(t *testing.T, nnodes, nshards int) ([]*Host, []raft.Transport) {
	t.Helper()
	nets := make([]raft.Transport, nshards)
	for i := 0; i < nshards; i++ {
		nets[i] = raft.NewMemory()
	}
	ids := make([]raft.ID, nnodes)
	for i := 0; i < nnodes; i++ {
		ids[i] = raft.ID(string(rune('a' + i)))
	}
	hosts := make([]*Host, nnodes)
	for i, id := range ids {
		h, err := StartHost(HostConfig{
			Dir:         t.TempDir(),
			Shards:      nshards,
			ID:          id,
			Peers:       ids,
			Sync:        storage.SyncNone,
			Transports:  nets,
			Heartbeat:   20 * time.Millisecond,
			ElectionMin: 50 * time.Millisecond,
			ElectionMax: 120 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		hosts[i] = h
	}
	return hosts, nets
}

func closeHosts(hosts []*Host) {
	for _, h := range hosts {
		h.Close()
	}
}

func waitShardLeader(t *testing.T, hosts []*Host, shardIdx int) *raft.Node {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var ls []*raft.Node
		for _, h := range hosts {
			if h == nil {
				continue
			}
			n := h.Replicas[shardIdx].Node
			if n.IsLeader() {
				ls = append(ls, n)
			}
		}
		if len(ls) == 1 {
			return ls[0]
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("shard %d: expected one leader", shardIdx)
	return nil
}

func keyOn(t *testing.T, rt *shard.Router, sid shard.ID) []byte {
	t.Helper()
	for i := 0; i < 20000; i++ {
		k := []byte("k" + strconv.Itoa(i))
		if rt.Owner(k) == sid {
			return k
		}
	}
	t.Fatalf("no key for shard %s", sid)
	return nil
}
