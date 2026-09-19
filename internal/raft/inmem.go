package raft

import (
	"sync"
	"time"
)

// Memory is an in-process Transport for tests.
type Memory struct {
	mu    sync.Mutex
	nodes map[ID]*Node
	gate  *Gate
	delay time.Duration
}

// NewMemory returns an empty in-memory network.
func NewMemory() *Memory {
	return &Memory{nodes: map[ID]*Node{}, gate: NewGate()}
}

// Gate is the partition controller for this network.
func (m *Memory) Gate() *Gate { return m.gate }

// Register adds a node.
func (m *Memory) Register(n *Node) {
	m.mu.Lock()
	m.nodes[n.id] = n
	m.mu.Unlock()
}

// Isolate drops all RPCs to/from id (for failover tests).
func (m *Memory) Isolate(id ID, on bool) {
	m.gate.Isolate(id, on)
}

func (m *Memory) node(to ID) *Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes[to]
}

// SendRequestVote implements Transport.
func (m *Memory) SendRequestVote(to ID, args RequestVoteArgs) (RequestVoteReply, error) {
	if m.gate.Blocked(args.Candidate, to) {
		return RequestVoteReply{}, errUnreachable
	}
	n := m.node(to)
	if n == nil {
		return RequestVoteReply{}, errUnreachable
	}
	return n.RequestVote(args), nil
}

// SendAppendEntries implements Transport.
func (m *Memory) SendAppendEntries(to ID, args AppendEntriesArgs) (AppendEntriesReply, error) {
	if m.gate.Blocked(args.Leader, to) {
		return AppendEntriesReply{}, errUnreachable
	}
	n := m.node(to)
	if n == nil {
		return AppendEntriesReply{}, errUnreachable
	}
	return n.AppendEntries(args), nil
}
