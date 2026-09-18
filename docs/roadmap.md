# NebulaDB Roadmap

This is a 3–6 month flagship project. Each phase has a **definition of done**. Later phases must not start until the previous phase's tests pass, including `-race` where concurrency is involved.

Work is committed **per step**, not as one giant dump. Typical commit sequence inside a phase:

1. Design notes (if the on-disk or wire format changed)
2. Package implementation + unit tests
3. Integration with the engine / CLI
4. Docs status update

---

## Phase 1 — Storage Engine (done)

**Goal:** Durable single-node KV: `Set`, `Get`, `Delete`.

### Steps

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 1.0 | Architecture, roadmap, ADRs | docs: project plan |
| 1.1 | Go module, Makefile, LICENSE, `.gitignore` | chore: scaffold |
| 1.2 | WAL: framed records, CRC32, `fsync`, replay | feat(storage): WAL |
| 1.3 | Skip-list MemTable + tombstones | feat(storage): memtable |
| 1.4 | Engine wiring: WAL-then-MemTable, Open/Close | feat(storage): engine |
| 1.5 | REPL (`nebuladb`) + crash-recovery tests | feat(cmd): REPL + recovery tests |

### Definition of done

- [x] `Set`/`Get`/`Delete` on byte keys/values
- [x] Every mutation appended to WAL before becoming visible
- [x] Configurable `fsync` (always / none)
- [x] Process restart reconstructs state from WAL
- [x] Truncated last record on crash does not corrupt the log
- [x] Concurrent writers pass `go test -race`

**Out of scope:** SSTables, SQL, network, Raft.

---

## Phase 2 — LSM Tree (done)

MemTable flush → immutable SSTables on disk → Bloom filters → compaction → tombstone GC.

See [docs/phases/phase-02-lsm.md](phases/phase-02-lsm.md).

## Phase 3 — Indexing (done)

Secondary indexes (LSM-backed) plus an in-memory B+ tree. See [docs/phases/phase-03-indexing.md](phases/phase-03-indexing.md).

SQL `CREATE INDEX` is Phase 4.

## Phase 4 — SQL Engine

Lexer → parser → AST → planner → executor over the KV/LSM engine.

Subset: `CREATE TABLE`, `INSERT`, `SELECT`, `UPDATE`, `DELETE`, `WHERE`, `ORDER BY`, `LIMIT`. Aggregates and `JOIN` after the subset is correct.

## Phase 5 — Transactions

`BEGIN` / `COMMIT` / `ROLLBACK`. Start with **Read Committed**, then Repeatable Read. Isolation tests with concurrent writers.

## Phase 6 — Raft

Three-node cluster, leader election, replicated log, majority commit, failover. Writes go only to the leader.

## Phase 7 — Sharding

Query router + consistent hashing. Rebalance is a later step inside this phase.

## Phase 8 — Replicated shards

Each shard is its own Raft group (leader + followers).

## Phase 9 — Failure injection

`nebulactl kill-node`, partitions, disk faults. Chaos tests that assert the guarantees in `consistency.md`.

## Phase 10 — Networking

gRPC for node-to-node; optional custom TCP for storage ops. Timeouts, retries, backpressure.

## Phase 11 — Observability

`/metrics` (Prometheus): QPS, latency histograms, WAL fsyncs, elections, compaction. Grafana dashboards.

## Phase 12 — Kubernetes

StatefulSet, PVCs, headless Service, probes. Rolling restart must not lose committed data.

## Phase 13 — Benchmarks + docs

Publish **measured** numbers only. `docs/benchmarks.md` with hardware, commit SHA, and method.

---

## What not to do

- Do not wrap PostgreSQL or etcd and call it NebulaDB.
- Do not implement HTTP CRUD as a substitute for the storage engine.
- Do not copy an entire Raft library into `internal/raft` without understanding and tests (write it, or document a comparison experiment).
- Do not write resume bullets for phases that are not implemented.
