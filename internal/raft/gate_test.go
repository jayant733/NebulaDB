package raft

import "testing"

func TestGateSplit(t *testing.T) {
	g := NewGate()
	g.Split([]ID{"a"}, true)
	if !g.Blocked("a", "b") {
		t.Fatal("expected split")
	}
	if g.Blocked("b", "c") {
		t.Fatal("majority should talk")
	}
	g.Heal()
	if g.Blocked("a", "b") {
		t.Fatal("healed")
	}
	g.Isolate("a", true)
	if !g.Blocked("a", "b") || !g.Blocked("c", "a") {
		t.Fatal("isolate")
	}
}
