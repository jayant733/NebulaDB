package raft

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

var (
	// ErrNotLeader is returned when a follower/candidate receives a client write.
	ErrNotLeader   = errors.New("raft: not leader")
	errUnreachable = errors.New("raft: unreachable")
)

// Role is the Raft server state.
type Role int

const (
	Follower Role = iota
	Candidate
	Leader
)

// Config constructs a Node.
type Config struct {
	ID          ID
	Peers       []ID // includes self
	Dir         string
	Apply       ApplyFunc
	Transport   Transport
	Heartbeat   time.Duration
	ElectionMin time.Duration
	ElectionMax time.Duration
}

// Node is one Raft server.
type Node struct {
	id        ID
	peers     []ID
	apply     ApplyFunc
	trans     Transport
	persist   *persistent
	heartbeat time.Duration
	elMin     time.Duration
	elMax     time.Duration

	mu            sync.Mutex
	role          Role
	commitIndex   uint64
	lastApplied   uint64
	nextIndex     map[ID]uint64
	matchIndex    map[ID]uint64
	leader        ID
	electionUntil time.Time
	lastHB        time.Time
	votes         int
	wait          map[uint64]chan error
	stop          chan struct{}
	stopped       bool
	rng           *rand.Rand
}

// Start loads persistent state and runs the ticker.
func Start(cfg Config) (*Node, error) {
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 50 * time.Millisecond
	}
	if cfg.ElectionMin == 0 {
		cfg.ElectionMin = 200 * time.Millisecond
	}
	if cfg.ElectionMax == 0 {
		cfg.ElectionMax = 400 * time.Millisecond
	}
	p, err := openPersistent(cfg.Dir)
	if err != nil {
		return nil, err
	}
	others := make([]ID, 0, len(cfg.Peers))
	for _, id := range cfg.Peers {
		if id != cfg.ID {
			others = append(others, id)
		}
	}
	n := &Node{
		id:         cfg.ID,
		peers:      others,
		apply:      cfg.Apply,
		trans:      cfg.Transport,
		persist:    p,
		heartbeat:  cfg.Heartbeat,
		elMin:      cfg.ElectionMin,
		elMax:      cfg.ElectionMax,
		role:       Follower,
		nextIndex:  map[ID]uint64{},
		matchIndex: map[ID]uint64{},
		wait:       map[uint64]chan error{},
		stop:       make(chan struct{}),
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	n.resetElectionLocked()
	go n.loop()
	return n, nil
}

// Stop ends the ticker. Safe to call twice.
func (n *Node) Stop() {
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return
	}
	n.stopped = true
	n.role = Follower
	close(n.stop)
	n.failWaitersLocked(ErrNotLeader)
	n.mu.Unlock()
}

// IsLeader reports whether this node thinks it is leader.
func (n *Node) IsLeader() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return !n.stopped && n.role == Leader
}

// ID is this server's stable identifier.
func (n *Node) ID() ID {
	return n.id
}

// LeaderID is the last known leader (may be empty).
func (n *Node) LeaderID() ID {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.leader
}

// Propose appends a command on the leader and waits until it is applied.
func (n *Node) Propose(cmd Command) error {
	n.mu.Lock()
	if n.role != Leader {
		n.mu.Unlock()
		return ErrNotLeader
	}
	e := n.appendLocked(cmd)
	ch := make(chan error, 1)
	n.wait[e.Index] = ch
	n.maybeCommitLocked()
	n.mu.Unlock()
	n.broadcast()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		return fmt.Errorf("raft: propose timeout")
	}
}

func (n *Node) loop() {
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-n.stop:
			return
		case <-t.C:
			n.tick()
		}
	}
}

func (n *Node) tick() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.stopped {
		return
	}
	now := time.Now()
	if n.role == Leader {
		if now.Sub(n.lastHB) >= n.heartbeat {
			n.lastHB = now
			go n.broadcast()
		}
		return
	}
	if now.After(n.electionUntil) {
		go n.startElection()
	}
}

func (n *Node) startElection() {
	n.mu.Lock()
	if n.stopped || n.role == Leader {
		n.mu.Unlock()
		return
	}
	n.role = Candidate
	n.persist.state.CurrentTerm++
	n.persist.state.VotedFor = n.id
	term := n.persist.state.CurrentTerm
	lastIdx, lastTerm := n.lastLocked()
	n.votes = 1
	n.leader = ""
	_ = n.persist.save()
	n.resetElectionLocked()
	others := append([]ID(nil), n.peers...)
	id := n.id
	majority := n.majority()
	n.mu.Unlock()

	if len(others) == 0 {
		n.mu.Lock()
		if n.persist.state.CurrentTerm == term && n.role == Candidate {
			n.becomeLeaderLocked()
		}
		n.mu.Unlock()
		return
	}
	for _, p := range others {
		p := p
		go func() {
			reply, err := n.trans.SendRequestVote(p, RequestVoteArgs{
				Term:         term,
				Candidate:    id,
				LastLogIndex: lastIdx,
				LastLogTerm:  lastTerm,
			})
			if err != nil {
				return
			}
			n.mu.Lock()
			defer n.mu.Unlock()
			if reply.Term > n.persist.state.CurrentTerm {
				n.becomeFollowerLocked(reply.Term)
				return
			}
			if n.role != Candidate || n.persist.state.CurrentTerm != term {
				return
			}
			if reply.VoteGranted {
				n.votes++
				if n.votes >= majority {
					n.becomeLeaderLocked()
				}
			}
		}()
	}
}

func (n *Node) majority() int {
	return (len(n.peers)+1)/2 + 1
}

func (n *Node) becomeLeaderLocked() {
	n.role = Leader
	n.leader = n.id
	last := n.lastIndexLocked()
	for _, p := range n.peers {
		n.nextIndex[p] = last + 1
		n.matchIndex[p] = 0
	}
	n.appendLocked(Command{Op: OpNoop})
	n.maybeCommitLocked()
	n.lastHB = time.Now()
	go n.broadcast()
}

func (n *Node) becomeFollowerLocked(term uint64) {
	if term > n.persist.state.CurrentTerm {
		n.persist.state.CurrentTerm = term
		n.persist.state.VotedFor = ""
		_ = n.persist.save()
	}
	if n.role == Leader {
		n.failWaitersLocked(ErrNotLeader)
	}
	n.role = Follower
}

func (n *Node) failWaitersLocked(err error) {
	for idx, ch := range n.wait {
		ch <- err
		delete(n.wait, idx)
	}
}

func (n *Node) resetElectionLocked() {
	span := n.elMax - n.elMin
	if span <= 0 {
		span = n.elMin
	}
	d := n.elMin + time.Duration(n.rng.Int63n(int64(span)+1))
	n.electionUntil = time.Now().Add(d)
}

func (n *Node) lastLocked() (idx, term uint64) {
	lg := n.persist.state.Log
	e := lg[len(lg)-1]
	return e.Index, e.Term
}

func (n *Node) lastIndexLocked() uint64 {
	idx, _ := n.lastLocked()
	return idx
}

func (n *Node) appendLocked(cmd Command) Entry {
	idx := n.lastIndexLocked() + 1
	e := Entry{
		Term:    n.persist.state.CurrentTerm,
		Index:   idx,
		Command: cloneCmd(cmd),
	}
	n.persist.state.Log = append(n.persist.state.Log, e)
	_ = n.persist.save()
	return e
}

func cloneCmd(c Command) Command {
	return Command{
		Op:    c.Op,
		Key:   append([]byte(nil), c.Key...),
		Value: append([]byte(nil), c.Value...),
	}
}

func cloneEntries(in []Entry) []Entry {
	out := make([]Entry, len(in))
	for i, e := range in {
		out[i] = Entry{Term: e.Term, Index: e.Index, Command: cloneCmd(e.Command)}
	}
	return out
}

func (n *Node) broadcast() {
	n.mu.Lock()
	if n.role != Leader || n.stopped {
		n.mu.Unlock()
		return
	}
	peers := append([]ID(nil), n.peers...)
	n.mu.Unlock()
	for _, p := range peers {
		p := p
		go n.replicate(p)
	}
}

func (n *Node) replicate(to ID) {
	n.mu.Lock()
	if n.role != Leader {
		n.mu.Unlock()
		return
	}
	next := n.nextIndex[to]
	if next == 0 {
		next = 1
	}
	log := n.persist.state.Log
	if next > uint64(len(log)-1)+0 /* last index */ {
		if next > log[len(log)-1].Index+1 {
			next = log[len(log)-1].Index + 1
			n.nextIndex[to] = next
		}
	}
	prev := next - 1
	if prev >= uint64(len(log)) {
		n.mu.Unlock()
		return
	}
	prevTerm := log[prev].Term
	var entries []Entry
	if next <= log[len(log)-1].Index {
		// log is indexed by slice position == Index for dummy-at-0
		start := int(next)
		if start < len(log) {
			entries = cloneEntries(log[start:])
		}
	}
	args := AppendEntriesArgs{
		Term:         n.persist.state.CurrentTerm,
		Leader:       n.id,
		PrevLogIndex: prev,
		PrevLogTerm:  prevTerm,
		Entries:      entries,
		LeaderCommit: n.commitIndex,
	}
	n.mu.Unlock()

	reply, err := n.trans.SendAppendEntries(to, args)
	if err != nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if reply.Term > n.persist.state.CurrentTerm {
		n.becomeFollowerLocked(reply.Term)
		return
	}
	if n.role != Leader {
		return
	}
	if reply.Success {
		if len(entries) > 0 {
			last := entries[len(entries)-1].Index
			n.nextIndex[to] = last + 1
			n.matchIndex[to] = last
		} else {
			n.matchIndex[to] = args.PrevLogIndex
			n.nextIndex[to] = args.PrevLogIndex + 1
		}
		n.maybeCommitLocked()
		return
	}
	if n.nextIndex[to] > 1 {
		n.nextIndex[to]--
	}
}

func (n *Node) maybeCommitLocked() {
	last := n.lastIndexLocked()
	for idx := last; idx > n.commitIndex; idx-- {
		if n.persist.state.Log[idx].Term != n.persist.state.CurrentTerm {
			continue
		}
		count := 1
		for _, p := range n.peers {
			if n.matchIndex[p] >= idx {
				count++
			}
		}
		if count >= n.majority() {
			n.commitIndex = idx
			n.applyCommittedLocked()
			return
		}
	}
}

func (n *Node) applyCommittedLocked() {
	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		e := n.persist.state.Log[n.lastApplied]
		if n.apply != nil && e.Command.Op != OpNoop {
			n.apply(e)
		}
		if ch, ok := n.wait[e.Index]; ok {
			ch <- nil
			delete(n.wait, e.Index)
		}
	}
}

// RequestVote is the incoming RPC.
func (n *Node) RequestVote(args RequestVoteArgs) RequestVoteReply {
	n.mu.Lock()
	defer n.mu.Unlock()
	if args.Term > n.persist.state.CurrentTerm {
		n.becomeFollowerLocked(args.Term)
	}
	reply := RequestVoteReply{Term: n.persist.state.CurrentTerm}
	if args.Term < n.persist.state.CurrentTerm {
		return reply
	}
	lastIdx, lastTerm := n.lastLocked()
	upToDate := args.LastLogTerm > lastTerm || (args.LastLogTerm == lastTerm && args.LastLogIndex >= lastIdx)
	canVote := n.persist.state.VotedFor == "" || n.persist.state.VotedFor == args.Candidate
	if canVote && upToDate {
		n.persist.state.VotedFor = args.Candidate
		_ = n.persist.save()
		n.resetElectionLocked()
		reply.VoteGranted = true
	}
	return reply
}

// AppendEntries is the incoming RPC.
func (n *Node) AppendEntries(args AppendEntriesArgs) AppendEntriesReply {
	n.mu.Lock()
	defer n.mu.Unlock()
	if args.Term > n.persist.state.CurrentTerm {
		n.becomeFollowerLocked(args.Term)
	}
	reply := AppendEntriesReply{Term: n.persist.state.CurrentTerm}
	if args.Term < n.persist.state.CurrentTerm {
		return reply
	}
	n.becomeFollowerLocked(args.Term)
	n.leader = args.Leader
	n.resetElectionLocked()

	log := n.persist.state.Log
	if args.PrevLogIndex >= uint64(len(log)) || log[args.PrevLogIndex].Term != args.PrevLogTerm {
		reply.Hint = uint64(len(log) - 1)
		return reply
	}
	insert := args.PrevLogIndex + 1
	if insert < uint64(len(log)) {
		n.persist.state.Log = append([]Entry{}, log[:insert]...)
	}
	if len(args.Entries) > 0 {
		n.persist.state.Log = append(n.persist.state.Log, cloneEntries(args.Entries)...)
	}
	_ = n.persist.save()
	if args.LeaderCommit > n.commitIndex {
		last := n.lastIndexLocked()
		n.commitIndex = args.LeaderCommit
		if n.commitIndex > last {
			n.commitIndex = last
		}
		n.applyCommittedLocked()
	}
	reply.Success = true
	return reply
}
