# Architecture

## What exists today (Phase 2)

```
  REPL / tests
       │
       ▼
  storage.Engine
       │
       ├── WAL (append-only; rotated after flush)
       ├── MemTable (skip list)
       └── SSTables (Bloom + restart index + compaction)
              │
              ▼
         Data directory
```

- The **WAL is the source of truth** on disk.
- Recent mutations live in the **MemTable**; older data lives in **SSTables**.
- The **WAL** records unflushed mutations. After flush, the WAL is rotated.
- On `Open`, the engine loads MANIFEST SSTables, then replays the WAL.
- There is no network, no SQL, and no replication.

## Target architecture (later phases)

```
                         ┌───────────────┐
                         │     Client    │
                         └───────┬───────┘
                                 │
                                 ▼
                         ┌───────────────┐
                         │ Query Router  │
                         └───────┬───────┘
                                 │
                     ┌───────────┼───────────┐
                     ▼           ▼           ▼
                  Shard 1     Shard 2     Shard 3
                  (Raft)      (Raft)      (Raft)
                     │           │           │
                    WAL         WAL         WAL
                     │           │           │
                  MemTable    MemTable    MemTable
                     │           │           │
                  SSTables    SSTables    SSTables
```

SQL, transactions, Raft, sharding, and Kubernetes layer **on top of** a correct storage engine. They do not replace it.

## Process model (Phase 2)

One OS process. One data directory:

```
<data>/
  MANIFEST         # ordered SSTable ids
  wal-000001.log   # unflushed mutations
  sst/000001.sst
```

Later: leveled compaction, `raft/` for consensus logs (the Raft log may *be* the WAL, or sit beside it — see ADRs when Phase 6 starts).

## Write path (Phase 1)

1. Encode mutation as a WAL record.
2. Append to the log file.
3. `fsync` if durability mode is `always`.
4. Apply to MemTable (Set or tombstone).
5. Return success to the caller.

Apply-after-log is required so a crash after fsync never loses a committed write, and a crash before fsync never exposes a write.

## Read path (Phase 2)

1. Lookup key in MemTable (tombstone → not found, stop).
2. Probe SSTables newest-to-oldest; Bloom miss skips a file.
3. First hit wins (put or tombstone).

## Concurrency

- WAL appends are serialized (single writer mutex) so records are not interleaved.
- MemTable supports concurrent reads/writes via a skip list + node-level atomics / mutex (see `internal/storage/memtable`).
- Engine `Get` does not take the WAL lock.

## Failure model (Phase 1)

| Failure | Behavior |
|---------|----------|
| Process kill after successful `Set` with sync=always | Restart replays WAL; key is present |
| Kill mid-append | Last incomplete record dropped; prior records intact |
| Disk full | Append returns error; MemTable is not updated |
| Bit flip in a record | CRC mismatch; recovery stops at that record (strict) |

Network partitions and split-brain are Phase 6+ concerns.
