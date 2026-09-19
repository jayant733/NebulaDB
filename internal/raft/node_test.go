package raft

import (
	"sync"
	"testing"
	"time"
)

func TestSingleNodePropose(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	got := []Command{}
	n, err := Start(Config{
		ID:        "n1",
		Peers:     []ID{"n1"},
		Dir:       dir,
		Transport: NewMemory(),
		Apply: func(e Entry) {
			mu.Lock()
			got = append(got, e.Command)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	waitLeader(t, n)
	if err := n.Propose(Command{Op: OpSet, Key: []byte("a"), Value: []byte("1")}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || string(got[0].Key) != "a" {
		t.Fatalf("%+v", got)
	}
}

func TestThreeNodeElectAndReplicate(t *testing.T) {
	net := NewMemory()
	type sm struct {
		mu   sync.Mutex
		data map[string]string
	}
	nodes := make([]*Node, 3)
	sms := make([]*sm, 3)
	ids := []ID{"a", "b", "c"}
	for i, id := range ids {
		sms[i] = &sm{data: map[string]string{}}
		sm := sms[i]
		n, err := Start(Config{
			ID:        id,
			Peers:     ids,
			Dir:       t.TempDir(),
			Transport: net,
			Apply: func(e Entry) {
				if e.Command.Op != OpSet {
					return
				}
				sm.mu.Lock()
				sm.data[string(e.Command.Key)] = string(e.Command.Value)
				sm.mu.Unlock()
			},
			ElectionMin: 50 * time.Millisecond,
			ElectionMax: 120 * time.Millisecond,
			Heartbeat:   20 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		net.Register(n)
		nodes[i] = n
		defer n.Stop()
	}
	lead := waitClusterLeader(t, nodes)
	if err := lead.Propose(Command{Op: OpSet, Key: []byte("k"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ok := true
		for _, s := range sms {
			s.mu.Lock()
			if s.data["k"] != "v" {
				ok = false
			}
			s.mu.Unlock()
		}
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("followers did not apply")
}

func TestLeaderFailover(t *testing.T) {
	net := NewMemory()
	ids := []ID{"a", "b", "c"}
	nodes := make([]*Node, 3)
	var mu sync.Mutex
	applied := 0
	for i, id := range ids {
		n, err := Start(Config{
			ID:        id,
			Peers:     ids,
			Dir:       t.TempDir(),
			Transport: net,
			Apply: func(e Entry) {
				if e.Command.Op == OpSet {
					mu.Lock()
					applied++
					mu.Unlock()
				}
			},
			ElectionMin: 50 * time.Millisecond,
			ElectionMax: 120 * time.Millisecond,
			Heartbeat:   20 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		net.Register(n)
		nodes[i] = n
		defer n.Stop()
	}
	old := waitClusterLeader(t, nodes)
	net.Isolate(old.id, true)
	old.Stop()

	var rest []*Node
	for _, n := range nodes {
		if n.id != old.id {
			rest = append(rest, n)
		}
	}
	lead := waitClusterLeader(t, rest)
	if err := lead.Propose(Command{Op: OpSet, Key: []byte("x"), Value: []byte("y")}); err != nil {
		t.Fatal(err)
	}
}

func waitLeader(t *testing.T, n *Node) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n.IsLeader() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no leader")
}

func waitClusterLeader(t *testing.T, nodes []*Node) *Node {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var ls []*Node
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
