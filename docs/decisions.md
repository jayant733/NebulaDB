# Architecture Decision Records

## ADR-001: Go, not Java/C++

**Status:** Accepted

Go gives goroutines, a strong standard library for files/networking, easy `-race`, and a resume gap relative to existing Spring/PostgreSQL work. C++ would be more "database-native" but slower to iterate; Java would overlap existing skills too much.

## ADR-002: LSM first, B+ tree later

**Status:** Accepted

Write-heavy WAL + MemTable + SSTables matches the crash-recovery story and scales to Raft log application. B+ trees are valuable for range reads and secondary indexes; they can land in Phase 3 without blocking Phase 1–2.

## ADR-003: Implement Raft in-tree (not etcd)

**Status:** Accepted for Phase 6

The point of the project is to *implement* leader election and log replication. Using etcd as the database would void the learning goal. A thin comparison benchmark against `hashicorp/raft` may be added later, clearly labeled as such.

## ADR-004: WAL-before-MemTable

**Status:** Accepted

If we updated memory first, a crash after Set returns but before fsync would lose a write the client was told succeeded — or we'd have to delay the client until fsync anyway, in which case the log should already contain the record.

## ADR-005: Strict CRC on recovery

**Status:** Accepted for Phase 1

A mismatched CRC fails recovery instead of skipping the record. Silent skip can hide disk bugs. A "skip and continue" salvage mode can be added behind a flag later.

## ADR-006: No HTTP as the internal protocol

**Status:** Accepted

REPL is fine for Phase 1. Cluster internals will use gRPC (Phase 10). HTTP may exist later for `/metrics` and a convenience SQL gateway, not for Raft.

## ADR-007: Resume bullets only for shipped, measured work

**Status:** Accepted

The marketing bullets in the original brief are a **target**, not the README. Update README status after each phase's definition of done.

## ADR-008: LSM secondary indexes as the durable default

**Status:** Accepted (Phase 3)

Secondary keys are ordinary LSM records, so they share WAL, flush, and crash recovery with primary rows. A separate B+ tree is implemented for split/range algorithms; it is in-memory in this phase. SQL `CREATE INDEX` will attach to the LSM catalog first.

## ADR-009: Hash the full LSM key; scatter-gather prefix scans

**Status:** Accepted (Phase 7)

Sharding by SQL primary key alone would split a row from its secondary-index entries and from the catalog unless every related key used the same routing id. Hashing the **entire LSM key** keeps the router a pure `storage.KV` and reuses SQL/index without a second key scheme.

Cost: `ScanPrefix` (table scans, index range, catalog backfill) must query every shard and merge. That is acceptable until the planner can push a single-key `Get`.

Replicated shards (one Raft group per shard) stay Phase 8 so Phase 7 can test placement and rebalance on one process.
