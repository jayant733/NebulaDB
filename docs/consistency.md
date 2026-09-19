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


## Intended model (Phase 6+ Raft)

- Writes succeed only on the **leader** after **majority** log replication.
- Followers never acknowledge a write to clients.
- On leader failure, a new leader is elected; committed entries are not lost (Raft).
- Uncommitted entries on a deposed leader may be overwritten — clients must retry.

Network partitions: a minority including an old leader cannot commit. This will be chaos-tested in Phase 9.

## What we will not claim until tested

- "CP in CAP" without partition tests
- Serializable isolation without a proof/tests
- Zero RPO across disks without fsync + Raft majority
