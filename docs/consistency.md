# Consistency and durability

This document states **what NebulaDB actually guarantees today**, then what later phases intend. Interview answers should match the current section, not the target.

## Phase 1 (single process)

### Durability

With `Sync: Always`, a `Set`/`Delete` that returned `nil` error has been `fsync`ed to the WAL. A crash after return will recover that mutation.

With `Sync: None`, durability is best-effort (OS buffer cache). Do not use this mode when claiming crash safety.

### Atomicity

Each KV mutation is one WAL record. There are no multi-key transactions yet. A crash cannot apply "half a record" (incomplete records are truncated).

### Isolation

No transactions. Concurrent `Get`/`Set` are race-detector clean. There is no snapshot isolation: a `Get` sees the latest applied MemTable value.

### Linearizability (single node)

For a given key, operations that have returned are totally ordered by WAL append order. Reads observe the last completed write.

### Replication

None. There is one copy of the log on local disk.

---

## Phase 5 (single process, transactions)

### Isolation

| Level | Behavior |
|-------|----------|
| Read Committed (default) | No dirty reads. Each statement sees the latest **committed** data plus the session's own writes. |
| Repeatable Read | Snapshot at `BEGIN`. Concurrent commits are invisible. `COMMIT` aborts on write-write conflict (`ErrConflict`). |

DDL is auto-commit only (not allowed inside an open transaction).

### Atomicity

A SQL transaction's writes are buffered until `COMMIT`. Other sessions do not see them. Crash mid-commit may persist a prefix of WAL records (see phase-05).


## Phase 6 (Raft, as implemented)

Writes succeed only on the **leader** after **majority** log replication. Followers return `ErrNotLeader`. After leader failure a new leader is elected; committed entries are not overwritten. Uncommitted entries on a deposed leader may be replaced.

Network partitions and split-brain tests are Phase 9.

## Phase 7 (local shards, as implemented)

Each key lives on exactly one local `storage.Engine` chosen by a consistent hash of the **full LSM key**. A point `Get`/`Set`/`Delete` touches that shard only. `ScanPrefix` merges every shard.

`COMMIT` still applies keys one at a time. A crash can leave a prefix of a multi-key transaction persisted **across several WALs**. There is no two-phase commit.

Adding a shard and calling `Rebalance` moves only remapped keys (set-then-delete). Until rebalance, lookups use the **new** ring, so keys that have not moved yet are not found — callers must rebalance after `Attach` before serving traffic.

## Phase 8 (Raft per shard, as implemented)

Each shard has its own Raft log and leader. A write hashes to one shard and is committed when a **majority of that shard's group** has the entry. Killing the leader of shard 0 does not prevent a new write on shard 1.

A `nebuladb` process proposes only if it leads the owning shard (`ErrNotLeader` otherwise). There is no automatic forward to the remote leader. Reads on a follower replica may lag.

Rebalance of keys while Raft groups are running is not implemented.

## What we will not claim until tested

- "CP in CAP" without partition tests
- Serializable isolation without a proof/tests
- Zero RPO across disks without fsync + Raft majority
