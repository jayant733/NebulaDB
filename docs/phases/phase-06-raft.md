# Phase 6 — Raft

In-tree Raft (ADR-003). Writes succeed only on the **leader** after a **majority** of nodes have the log entry.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 6.0 | Protocol and membership | `docs: specify Raft` |
| 6.1 | Persistent log + term/vote | `feat(raft): persistent log` |
| 6.2 | Election + AppendEntries (in-memory transport) | `feat(raft): leader election and replication` |
| 6.3 | Propose/apply + failover tests | `feat(raft): commit and apply` |
| 6.4 | TCP RPC transport | `feat(raft): TCP transport` |
| 6.5 | Cluster-mode `nebuladb` | `feat(cmd): Raft cluster` |

## Definition of done

- Three-node cluster elects one leader
- `Propose` on the leader commits after majority `AppendEntries` success
- Followers apply the same log in order
- Killing the leader triggers a new election; a new `Propose` succeeds
- Followers reject client writes (`ErrNotLeader`)
- Persistent `currentTerm`, `votedFor`, and log survive process restart

## RPCs

**RequestVote** — candidate term, last log index/term. Grant if term is current, vote is free, and candidate log is at least as up-to-date.

**AppendEntries** — prev log index/term, entries, leader commit. Heartbeats are empty AppendEntries.

## Apply path

```
Client write (leader)
  → append log
  → replicate to followers
  → majority matchIndex
  → commitIndex advances
  → apply to local LSM Engine
```

A no-op entry is appended when a node becomes leader so it can commit leftover previous-term entries (Raft §5.4.2).

gRPC and full membership changes are Phase 10 / later. Phase 6 uses `net/rpc` over TCP and a static peer list.
