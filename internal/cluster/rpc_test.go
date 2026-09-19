package cluster

import (
	"net"
	"testing"

	"github.com/jayant/nebuladb/internal/raft"
)

func TestClientForwardToLeader(t *testing.T) {
	hosts, _ := startHosts(t, 3, 2)
	defer closeHosts(hosts)
	waitShardLeader(t, hosts, 0)
	waitShardLeader(t, hosts, 1)

	addrs := map[raft.ID]string{}
	lns := map[raft.ID]net.Listener{}
	for _, h := range hosts {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		lns[h.ID] = ln
		addrs[h.ID] = ln.Addr().String()
	}
	for _, h := range hosts {
		if err := h.ServeKV(lns[h.ID], addrs); err != nil {
			t.Fatal(err)
		}
	}

	rt, err := LeaderRouter(hosts)
	if err != nil {
		t.Fatal(err)
	}
	k := keyOn(t, rt, "0")
	var follow *Host
	for _, h := range hosts {
		if !h.Replicas[0].Node.IsLeader() {
			follow = h
			break
		}
	}
	if follow == nil {
		t.Fatal("no follower")
	}
	cli := &Client{Addrs: []string{addrs[follow.ID]}}
	if err := cli.Set(k, []byte("fwd")); err != nil {
		t.Fatal(err)
	}
	v, ok, err := cli.Get(k)
	if err != nil || !ok || string(v) != "fwd" {
		t.Fatalf("get %q %v %v", v, ok, err)
	}
}
