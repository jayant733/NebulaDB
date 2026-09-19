package raft

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestTCPThreeNodePropose(t *testing.T) {
	ids := []ID{"a", "b", "c"}
	lns := make([]net.Listener, 3)
	addrs := map[ID]string{}
	for i, id := range ids {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		lns[i] = ln
		addrs[id] = ln.Addr().String()
	}
	trans := NewTCP(addrs)
	nodes := make([]*Node, 3)
	var mu sync.Mutex
	applied := make([]map[string]string, 3)
	for i, id := range ids {
		applied[i] = map[string]string{}
		sm := applied[i]
		n, err := Start(Config{
			ID:        id,
			Peers:     ids,
			Dir:       t.TempDir(),
			Transport: trans,
			Apply: func(e Entry) {
				if e.Command.Op == OpSet {
					mu.Lock()
					sm[string(e.Command.Key)] = string(e.Command.Value)
					mu.Unlock()
				}
			},
			ElectionMin: 80 * time.Millisecond,
			ElectionMax: 160 * time.Millisecond,
			Heartbeat:   30 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := Serve(n, lns[i]); err != nil {
			t.Fatal(err)
		}
		nodes[i] = n
		defer n.Stop()
	}
	lead := waitClusterLeader(t, nodes)
	if err := lead.Propose(Command{Op: OpSet, Key: []byte("k"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ok := true
		mu.Lock()
		for _, m := range applied {
			if m["k"] != "v" {
				ok = false
			}
		}
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("tcp followers did not apply")
}
