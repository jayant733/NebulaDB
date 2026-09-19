# Architecture

## What exists today (Phase 10)

Three process modes:

1. **Single engine** (`--shards 1`, no `--peers`): SQL → one LSM.
2. **One Raft group** (`--shards 1 --peers`): SQL → replicated LSM (Phase 6).
3. **Shards** (`--shards N`, N≥2): SQL → `shard.Router` → N engines. With `--peers`, each shard is its own Raft group (ports `base+shardIndex`). Without `--peers`, shards are local only (Phase 7).

Writes on a process succeed only when that process is Raft leader for the hashed shard, **or** the client uses KV RPC which forwards to that leader. `nebulactl` talks to the same RPC port for isolate/heal/disk-fail/kill-node.

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

Raft persistent state is `<data>/raft/state.gob` (term, vote, log). The LSM WAL is still the local apply log, not the Raft log.

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

Network partitions and split-brain tests are Phase 9. Phase 6 elects a leader and replicates on a static peer list.
