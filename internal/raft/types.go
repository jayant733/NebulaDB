package raft

// ID is a node identifier (stable, not an address).
type ID string

// Command is a replicated state-machine operation.
type Command struct {
	Op    uint8 // 1 = Set, 2 = Delete
	Key   []byte
	Value []byte
}

const (
	OpNoop   uint8 = 0
	OpSet    uint8 = 1
	OpDelete uint8 = 2
)

// Entry is one Raft log record. Index is 1-based; index 0 is a dummy.
type Entry struct {
	Term    uint64
	Index   uint64
	Command Command
}

// RequestVoteArgs is sent by candidates.
type RequestVoteArgs struct {
	Term         uint64
	Candidate    ID
	LastLogIndex uint64
	LastLogTerm  uint64
}

// RequestVoteReply is the vote response.
type RequestVoteReply struct {
	Term        uint64
	VoteGranted bool
}

// AppendEntriesArgs is sent by the leader (heartbeats have empty Entries).
type AppendEntriesArgs struct {
	Term         uint64
	Leader       ID
	PrevLogIndex uint64
	PrevLogTerm  uint64
	Entries      []Entry
	LeaderCommit uint64
}

// AppendEntriesReply is the replication response.
type AppendEntriesReply struct {
	Term    uint64
	Success bool
	// Hint is a follower log length to speed retries (optional).
	Hint uint64
}

// ApplyFunc is invoked in log order on the state machine.
type ApplyFunc func(Entry)

// Transport sends RPCs to peers.
type Transport interface {
	SendRequestVote(to ID, args RequestVoteArgs) (RequestVoteReply, error)
	SendAppendEntries(to ID, args AppendEntriesArgs) (AppendEntriesReply, error)
}
