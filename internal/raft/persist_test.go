package raft

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := openPersistent(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.state.CurrentTerm = 3
	p.state.VotedFor = "n2"
	p.state.Log = append(p.state.Log, Entry{Term: 3, Index: 1, Command: Command{Op: OpSet, Key: []byte("a"), Value: []byte("b")}})
	if err := p.save(); err != nil {
		t.Fatal(err)
	}
	p2, err := openPersistent(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p2.state.CurrentTerm != 3 || p2.state.VotedFor != "n2" || len(p2.state.Log) != 2 {
		t.Fatalf("%+v", p2.state)
	}
	if string(p2.state.Log[1].Command.Key) != "a" {
		t.Fatal("log payload")
	}
	if _, err := os.Stat(filepath.Join(dir, "state.gob")); err != nil {
		t.Fatal(err)
	}
}
