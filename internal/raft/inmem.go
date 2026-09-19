package raft

import (
	"sync"
	"time"
)

// Memory is an in-process Transport for tests.
type Memory struct {
	mu    sync.Mutex
	nodes map[ID]*Node
	drop  map[ID]bool
	delay time.Duration
}

// NewMemory returns an empty in-memory network.
func NewMemory() *Memory {
	return &Memory{nodes: map[ID]*Node{}, drop: map[ID]bool{}}
}

// Register adds a node.
func (m *Memory) Register(n *Node) {
	m.mu.Lock()
	m.nodes[n.id] = n
	m.mu.Unlock()
}

// Isolate drops all RPCs to/from id (for failover tests).
func (m *Memory) Isolate(id ID, on bool) {
	m.mu.Lock()
	m.drop[id] = on
	m.mu.Unlock()
}

func (m *Memory) blocked(to ID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.drop[to]
}

func (m *Memory) node(to ID) *Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes[to]
}

// SendRequestVote implements Transport.
func (m *Memory) SendRequestVote(to ID, args RequestVoteArgs) (RequestVoteReply, error) {
	if m.blocked(to) || m.blocked(args.Candidate) {
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
	if m.blocked(to) || m.blocked(args.Leader) {
		return AppendEntriesReply{}, errUnreachable
	}
	n := m.node(to)
	if n == nil {
		return AppendEntriesReply{}, errUnreachable
	}
	return n.AppendEntries(args), nil
}
