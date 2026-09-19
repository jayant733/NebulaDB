package cluster

import (
	"testing"

	"github.com/jayant/nebuladb/internal/raft"
)

func TestChaosMajorityStillCommits(t *testing.T) {
	hosts, nets := startHosts(t, 3, 2)
	defer closeHosts(hosts)
	lead := waitShardLeader(t, hosts, 0)
	var follower raft.ID
	for _, h := range hosts {
		id := h.Replicas[0].Node.ID()
		if id != lead.ID() {
			follower = id
			break
		}
	}
	nets[0].(*raft.Memory).Isolate(follower, true)
	rt, err := LeaderRouter(hosts)
	if err != nil {
		t.Fatal(err)
	}
	k := keyOn(t, rt, "0")
	if err := rt.Set(k, []byte("ok")); err != nil {
		t.Fatal(err)
	}
}

func TestChaosMinorityLeaderNoSplitBrain(t *testing.T) {
	hosts, nets := startHosts(t, 3, 2)
	defer closeHosts(hosts)
	old := waitShardLeader(t, hosts, 0)
	nets[0].(*raft.Memory).Gate().Split([]raft.ID{old.ID()}, true)

	var majority []*Host
	for _, h := range hosts {
		if h.Replicas[0].Node.ID() != old.ID() {
			majority = append(majority, h)
		}
	}
	waitShardLeader(t, majority, 0)
	rt, err := LeaderRouter(majority)
	if err != nil {
		t.Fatal(err)
	}
	k := keyOn(t, rt, "0")
	if err := rt.Set(k, []byte("maj")); err != nil {
		t.Fatal(err)
	}
	if err := old.Propose(raft.Command{Op: raft.OpSet, Key: k, Value: []byte("min")}); err == nil {
		t.Fatal("minority leader committed")
	}
	v, ok, err := rt.Get(k)
	if err != nil || !ok || string(v) != "maj" {
		t.Fatalf("majority value %q %v %v", v, ok, err)
	}
}

