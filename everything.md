# NebulaDB — Distributed Storage & SQL Engine: Technical Interview Master Guide

> **Document Type:** Systems Engineering Architecture, Database Internals & Senior Technical Interview Preparation Guide  
> **Repository:** `NebulaDB` (`jayant733/NebulaDB`)  
> **Author & Systems Engineer:** Core Database & Infrastructure Team  
> **Language & Stack:** Go (1.22+), Python (Analytics / PySpark), Docker, Kubernetes, Prometheus, Grafana  
> **Core Architectural Pillars:** Write-Ahead Logging (WAL), Log-Structured Merge-Tree (LSM), In-Tree Raft Consensus, Consistent Hashing Sharding, Transactional SQL Engine, Chaos Injection  

---

## Table of Contents
1. [Executive Summary & Project Philosophy](#1-executive-summary--project-philosophy)
2. [End-to-End System Architecture & Topological Breakdown](#2-end-to-end-system-architecture--topological-breakdown)
3. [Storage Engine Deep-Dive: WAL, MemTable, SSTable & Compaction](#3-storage-engine-deep-dive-wal-memtable-sstable--compaction)
4. [Relational Schema, Key Encoding & SQL Query Engine](#4-relational-schema-key-encoding--sql-query-engine)
5. [Transactions & Concurrency Control (ACID Semantics)](#5-transactions--concurrency-control-acid-semantics)
6. [Distributed Consensus: The In-Tree Raft Implementation](#6-distributed-consensus-the-in-tree-raft-implementation)
7. [Horizontal Sharding & Multi-Raft Architecture](#7-horizontal-sharding--multi-raft-architecture)
8. [Networking, Client RPC & Chaos Fault Injection](#8-networking-client-rpc--chaos-fault-injection)
9. [Observability, Metrics & Kubernetes Cloud-Native Deployment](#9-observability-metrics--kubernetes-cloud-native-deployment)
10. [Analytics Layer, Star Schema & Python Data Engineering](#10-analytics-layer-star-schema--python-data-engineering)
11. [The Master Metric & Benchmark Ledger (All Verified Numbers)](#11-the-master-metric--benchmark-ledger-all-verified-numbers)
12. ["Why This Instead of That?" — Critical Architectural Trade-Offs (ADRs)](#12-why-this-instead-of-that--critical-architectural-trade-offs-adrs)
13. [Future Systems Roadmap & Scalability Enhancements](#13-future-systems-roadmap--scalability-enhancements)
14. [Master Technical Interview Preparation: Questions & Senior Model Answers](#14-master-technical-interview-preparation-questions--senior-model-answers)
15. [Quick Interview Talking Points & High-Impact Summary](#15-quick-interview-talking-points--high-impact-summary)

---

## 1. Executive Summary & Project Philosophy

### 1.1 What is NebulaDB?
NebulaDB is a distributed, fault-tolerant, horizontally scalable SQL database engine built **from scratch in Go**. It is **not** a wrapper around PostgreSQL, SQLite, RocksDB, or etcd. Every foundational layer of database systems engineering—from raw append-only binary disk framing, skip-list memory structures, and Bloom filter math, to a recursive-descent SQL parser, an in-tree implementation of the Raft consensus protocol, Ketama consistent hash rings, and chaos injection gates—has been designed and implemented as first-class code.

### 1.2 The Guiding Principles
The development of NebulaDB followed four strict engineering tenets:
1. **Implement Internals:** True database engineering requires building storage, crash recovery, and consensus from basic operating system primitives (`os.File`, `net.Listener`, `sync.Mutex`, goroutines).
2. **Prove, Don't Advertise:** Performance claims, queries per second (QPS), and latency percentiles (p50, p99) must be derived from verified, reproducible microbenchmarks (`cmd/bench`), not speculative marketing numbers.
3. **Correctness Before Distribution:** A distributed system built on a flawed single-node storage engine is merely a distributed bug generator. Single-node crash recovery, WAL idempotency, and race-free data access (`go test -race`) precede network clustering.
4. **Independently Demoable Milestones:** Every phase of the architecture—from a single-node WAL-backed key-value store to multi-shard Raft clusters with fault injection—meets explicit, testable definitions of done.

---

## 2. End-to-End System Architecture & Topological Breakdown

NebulaDB is structured as a layered, modular database system:

```
                            ┌────────────────────────────────────────┐
                            │               SQL Client               │
                            │      (REPL / Python ETL / Apps)        │
                            └───────────────────┬────────────────────┘
                                                │ TCP / SQL Text or net/rpc
                                                ▼
                            ┌────────────────────────────────────────┐
                            │           Client KV RPC Layer          │
                            │  (Forward-to-Leader, Inflight Limit=32)│
                            └───────────────────┬────────────────────┘
                                                │
                                                ▼
                            ┌────────────────────────────────────────┐
                            │          Query Router (Shard)          │
                            │      Consistent Hash Ring (Ketama)     │
                            │          64 Virtual Nodes / Shard      │
                            └───────┬────────────────────────┬───────┘
                                    │                        │
                      Shard 0 Route │                        │ Shard 1 Route
                                    ▼                        ▼
     ┌────────────────────────────────────────┐   ┌────────────────────────────────────────┐
     │              Node 1 (Host)             │   │              Node 2 (Host)             │
     │  ┌─────────────────┐ ┌───────────────┐ │   │  ┌─────────────────┐ ┌───────────────┐ │
     │  │  Shard 0 Leader │ │Shard 1 Follower │   │  │Shard 0 Follower │ │ Shard 1 Leader  │ │
     │  └────────┬────────┘ └───────┬───────┘ │   │  └────────┬────────┘ └───────┬───────┘ │
     └───────────┼──────────────────┼─────────┘   └───────────┼──────────────────┼─────────┘
                 │ Raft RPC (:7100) │ Raft RPC (:7101)        │                  │
                 └──────────────────┼─────────────────────────┘                  │
                                    └────────────────────────────────────────────┘
                                                │
                                                ▼ Replicated Commit to Local Engine
                                    ┌───────────────────────┐
                                    │     Storage Engine    │
                                    │ ┌───────────────────┐ │
                                    │ │  Append-Only WAL  │ │ (SyncAlways: fsync per write)
                                    │ └─────────┬─────────┘ │
                                    │           ▼           │
                                    │ ┌───────────────────┐ │
                                    │ │ SkipList MemTable │ │ (Height ~ log4 N, 1 MiB limit)
                                    │ └─────────┬─────────┘ │
                                    │           ▼ Flush     │
                                    │ ┌───────────────────┐ │
                                    │ │ Immutable SSTable │ │ (Bloom 1% FPR, 4 KiB Block Index)
                                    │ └─────────┬─────────┘ │
                                    │           ▼           │
                                    │ ┌───────────────────┐ │
                                    │ │Size-Tiered Compact│ │ (Merge N>=4 SSTables, Drop Tombs)
                                    │ └───────────────────┘ │
                                    └───────────────────────┘
```

### 2.1 The 14 Completed Phases of the NebulaDB Engine
- **Phase 1 (Storage Engine):** Durable single-node KV (`Set`, `Get`, `Delete`), CRC32-framed WAL, skip-list MemTable, `SyncAlways` vs `SyncNone`.
- **Phase 2 (LSM Tree):** MemTable flush, immutable SSTables, 4 KiB restart block indexing, Bloom filters (1% FPR), size-tiered compaction ($N \ge 4$), `MANIFEST` file.
- **Phase 3 (Indexing):** Durable LSM-backed secondary indexes with order-preserving byte encoding + in-memory B+ tree implementation.
- **Phase 4 (SQL Engine):** Lexer, recursive-descent Parser, AST, catalog, execution engine for `CREATE TABLE`, `INSERT`, `SELECT`, `UPDATE`, `DELETE`, `WHERE`, `ORDER BY`, `LIMIT`.
- **Phase 5 (Transactions):** Single-node ACID transactions (`BEGIN`, `COMMIT`, `ROLLBACK`) with Read Committed and Repeatable Read (optimistic conflict abort).
- **Phase 6 (Raft Consensus):** In-tree Raft protocol with persistent term/vote/log state, leader election, log replication, majority commit, and automatic failover.
- **Phase 7 (Sharding):** Consistent hash ring (Ketama with 64 vnodes) routing full LSM keys across local storage engines with scatter-gather prefix scans and key rebalancing.
- **Phase 8 (Replicated Shards):** Multi-Raft architecture. Each shard runs its own isolated Raft consensus group across physical nodes on offset ports (`base + shardIndex`).
- **Phase 9 (Failure Injection):** Chaos testing harness and `nebulactl` admin CLI for network partitions (`isolate`, `heal`), node kills, and disk I/O errors (`storage.ErrDisk`).
- **Phase 10 (Networking):** Client KV RPC (`net/rpc` over TCP) with automatic forwarding to the active shard leader, call timeouts (2s), retries, and inflight backpressure (32 max).
- **Phase 11 (Observability):** Zero-external-dependency Prometheus text exposition (`/metrics`), latency histograms, operation counters, and Grafana dashboard templates.
- **Phase 12 (Analytics SQL & Python ETL):** Relational nested-loop `INNER JOIN ... ON`, `GROUP BY` aggregates (`COUNT`, `SUM`, `AVG`), Python data cleaning pipeline, star schema export (`dim_customer`, `fact_order`), PySpark job (`spark_job.py`), and natural-language-to-SQL stub.
- **Phase 13 (Kubernetes Deployment):** Dockerfile, StatefulSet with PersistentVolumeClaims (`volumeClaimTemplates`), headless Service for stable DNS, HTTP `/livez` and `/readyz` probes, and graceful SIGTERM shutdown with WAL sync.
- **Phase 14 (Microbenchmarks):** Rigorous, reproducible benchmarking CLI (`cmd/bench`) measuring raw QPS, mean latency, and p99 percentiles across durability modes.

---

## 3. Storage Engine Deep-Dive: WAL, MemTable, SSTable & Compaction

The foundation of NebulaDB is a Log-Structured Merge-Tree (LSM) storage engine located in `internal/storage/`.

### 3.1 Write-Ahead Log (WAL)
Every mutation (`Put` or `Delete`) is appended to disk in an append-only log file (`wal-000001.log`) before it is applied to the in-memory MemTable.

#### WAL Binary On-Disk Frame
Each record is encoded using little-endian binary serialization:
```
+----------------+---------------+---------------+---------------+--------------------+----------------------+
|  crc32 (4 B)   |  type (1 B)   |  klen (4 B)   |  vlen (4 B)   |     key (klen B)   |     value (vlen B)   |
+----------------+---------------+---------------+---------------+--------------------+----------------------+
```
- **Header Size:** Exactly **13 bytes** (`4 + 1 + 4 + 4`).
- **`crc32` (uint32):** IEEE standard CRC-32 checksum calculated over the slice `[type .. payload]`. The checksum itself is excluded from the calculation.
- **`type` (uint8):** `1` represents `RecPut`; `2` represents `RecDelete` (tombstone).
- **`klen` / `vlen` (uint32):** Length-prefixed keys and values, allowing arbitrary binary payloads (including null bytes `0x00`) without delimiter collision.

#### Durability Modes
- **`SyncAlways` (Default):** Calls `os.File.Sync()` (`fsync`) after every single write operation. Guarantees zero data loss (RPO = 0) upon abrupt power failure or OS crash.
- **`SyncNone`:** Relies exclusively on the OS page cache. Used for high-throughput benchmarking; mutations remain vulnerable to uncommitted OS cache loss.

#### Crash Recovery & Idempotent Replay
During `storage.Open()`, the recovery sequence:
1. Seeks to the start of `wal-000001.log`.
2. Reads records sequentially. If an incomplete record header or truncated payload is encountered (a torn write caused by an OS crash mid-write), the WAL is automatically truncated back to the last known good byte offset.
3. Computes the IEEE CRC32 of each read payload. If a checksum mismatch occurs, recovery immediately terminates with an error rather than silently ignoring corruption (ADR-005).
4. Replays all valid operations into a fresh MemTable in exact append order.

### 3.2 MemTable (Concurrent SkipList)
The MemTable (`internal/storage/memtable/`) is an in-memory sorted data structure based on a probabilistic **SkipList**:
- **Ordering:** Strictly ordered using lexicographical byte comparison (`bytes.Compare(keyA, keyB)`).
- **Search Complexity:** $O(\log N)$ average time complexity for reads and inserts.
- **Branching Factor:** Uses a geometric distribution with probability $p = 0.25$, yielding an expected tree height proportional to $\log_4(N)$, with a maximum height cap of 16 levels.
- **Tombstones:** Deletions insert an explicit tombstone entry (`deleted = true`, empty value), preventing older values in SSTables on disk from surfacing.
- **Memory Threshold:** Default flush threshold is **1 MiB** (`1 << 20` bytes). When the MemTable reaches this capacity, it is frozen and flushed to disk as an SSTable.

### 3.3 SSTable (Sorted String Table) Format
SSTables (`internal/storage/sstable/`) are immutable on-disk files storing sorted key-value pairs with built-in indexing and probabilistic filtering.

```
┌─────────────────────────────────────────────────────────────┐
│ DATA BLOCK: Sequential sorted records                       │
│   [type: 1B][klen: 4B][vlen: 4B][key bytes][value bytes]    │
│   ...                                                       │
├─────────────────────────────────────────────────────────────┤
│ INDEX BLOCK: Two-level restart offsets (every 4 KiB)        │
│   count (uint32)                                            │
│   repeating: [offset: uint64][klen: uint32][key bytes]      │
├─────────────────────────────────────────────────────────────┤
│ BLOOM FILTER: Encoded bit array for 1% False Positive Rate  │
│   [k: 1B][nbits: 4B][bits: ((nbits+7)/8) bytes]             │
├─────────────────────────────────────────────────────────────┤
│ FOOTER: Fixed 24 bytes at EOF                               │
│   index_off (uint64) - 8 bytes                              │
│   bloom_off (uint64) - 8 bytes                              │
│   magic (0x4C535431 "LST1") - 4 bytes                       │
│   crc32 (IEEE checksum of preceding 20 bytes) - 4 bytes     │
└─────────────────────────────────────────────────────────────┘
```

#### Key Mechanics:
- **Restart Key Indexing:** Every **4,096 bytes (4 KiB)** of data written, a restart point records the current byte offset and key. A point lookup reads the index block first, performs a binary search to identify the candidate 4 KiB block, and scans only that small window.
- **Bloom Filter:** Sized using standard optimal formulas for an intended **1% False Positive Rate ($\mathbf{p = 0.01}$)**:
  $$m = -\frac{n \ln(p)}{(\ln 2)^2} \approx 9.58 \times n \text{ bits}, \quad k = \frac{m}{n} \ln 2 \approx 7 \text{ hash functions}$$
  If the Bloom filter returns negative, the file is bypassed immediately without touching disk data blocks.
- **Fixed Footer:** Always located at the final 24 bytes of the file. It contains the offsets of the index block and Bloom filter, verified by a magic byte sequence (`0x4C535431`, ASCII `"LST1"`) and a CRC32 checksum.

### 3.4 Read Path Hierarchy
```
Client Get(key)
      │
      ▼
1. Query Active MemTable ───────────► Found? ──► Return Value / Tombstone
      │ (Miss)
      ▼
2. Read MANIFEST (Order: Newest SSTable -> Oldest SSTable)
      │
      ▼
3. For each SSTable Reader:
      │
      ├─► Check In-Memory Bloom Filter ─► Negative? ──► Skip to Next SSTable
      │                                       │ (Positive)
      ├─► Binary Search 4 KiB Index Block ◄───┘
      │
      └─► Seek & Scan 4 KiB Data Window ──► Found? ──► Return Value / Tombstone
      │ (Miss across all SSTables)
      ▼
4. Return (nil, false, nil) [Key Not Found]
```

### 3.5 Manifest & Size-Tiered Compaction
- **`MANIFEST`:** An append-only text file in the data directory recording active SSTable IDs in chronological order (`sst 000001`, `sst 000002`). Reads search from the bottom (newest) upwards.
- **Compaction Strategy:** NebulaDB employs size-tiered compaction. When the count of live SSTables reaches **`CompactN` (default: 4)**, a compaction cycle initiates:
  1. Opens an iterator across all active SSTables concurrently.
  2. Executes an $N$-way merge-sort where newer SSTable keys override older versions.
  3. Purges deleted keys (tombstones) permanently from disk since all historical levels are being collapsed into a single consolidated file.
  4. Writes the merged output to a new SSTable, atomically rewrites the `MANIFEST`, and unlinks the superseded SSTable files.

---

## 4. Relational Schema, Key Encoding & SQL Query Engine

NebulaDB layers a complete relational SQL database over its raw key-value storage engine using clean schema mapping and order-preserving encoding (`internal/row/`, `internal/index/`, `internal/sql/`).

### 4.1 Relational-to-KV Mapping
All database entities—tables, rows, indexes, and catalogs—are stored as keys in the unified LSM key space:

| Entity Type | Key Format | Value Format |
|---|---|---|
| **Catalog Schema** | `x\x00catalog\x00<table_name>` | JSON-serialized schema definition |
| **Primary Row** | `p<table_name>\x00<pk_bytes>` | Length-prefixed encoded row fields |
| **Secondary Index** | `i<table_name>\x00<index_name>\x00<encoded_sk><pk_bytes>` | Empty byte array (`nil`) |

### 4.2 Order-Preserving Comparable Byte Encoding
In secondary indexes, SQL values (integers, strings) must sort correctly in the byte-ordered LSM tree. NebulaDB implements an **order-preserving escape algorithm** (`internal/index/keys.go`):
- Any null byte `0x00` inside a key value is escaped to the two-byte sequence `0x00 0xFF`.
- The end of the secondary key component is marked with the terminator sequence `0x00 0x01`.
- Because `0x01 < 0xFF`, keys with a common prefix sort before keys containing escaped zero bytes, preserving strict lexicographical sorting while preventing delimiter collision.

### 4.3 Row Serialization (`internal/row/row.go`)
Row tuples are serialized as little-endian length-prefixed binary buffers:
`[field_count: uint32][len_1: uint32][field_1_bytes]...[len_n: uint32][field_n_bytes]`
This enables $O(1)$ sequential field extraction and avoids string parsing overhead.

### 4.4 SQL Parser & Execution Pipeline
The SQL engine (`internal/sql/`) implements:
1. **Lexer (`lexer/`):** Tokenizes incoming SQL text into typed tokens (`TokenSelect`, `TokenFrom`, `TokenJoin`, `TokenIdent`, `TokenNumber`).
2. **Parser (`parser/`):** A hand-written recursive-descent parser constructing an Abstract Syntax Tree (AST) without external parser generators like Yacc/Bison.
3. **Engine Execution (`engine/`):**
   - **Point Lookups:** If a query contains `WHERE pk = constant`, the planner routes directly to a single `Get(PrimaryKey(table, pk))` operation.
   - **Secondary Index Scans:** Queries with `WHERE indexed_col = val` execute an index prefix scan (`SecondaryKey(table, index, val, nil)`), extracting primary keys and performing point lookups on the base table.
   - **Table Scans:** Table scans use `ScanPrefix(PrimaryPrefix(table))` to iterate through rows.
   - **Nested-Loop Join:** Implements `INNER JOIN ... ON tableA.col = tableB.col` by scanning the outer table and performing matching inner-table evaluations in memory.
   - **Aggregations & Grouping:** Evaluates `COUNT(*)`, `COUNT(col)`, `SUM(col)`, `AVG(col)` grouped by target columns using hash tables during projection.

---

## 5. Transactions & Concurrency Control (ACID Semantics)

NebulaDB supports multi-statement transactional semantics (`internal/storage/txn.go`) with configurable isolation levels.

### 5.1 Supported Syntax
```sql
BEGIN [TRANSACTION] [READ COMMITTED | REPEATABLE READ];
INSERT INTO accounts VALUES (1, 'Alice', 500);
UPDATE accounts SET balance = balance - 100 WHERE id = 1;
COMMIT [TRANSACTION];
-- or ROLLBACK [TRANSACTION];
```

### 5.2 Isolation Levels & Mechanics

| Isolation Level | Dirty Reads? | Non-Repeatable Reads? | Phantom Reads? | Implementation Mechanism |
|---|:---:|:---:|:---:|---|
| **Read Committed** *(Default)* | **No** | Yes | Yes | Uncommitted writes are buffered in private session memory. Readers see the latest committed engine state plus their own uncommitted local writes. |
| **Repeatable Read** | **No** | **No** | Prevented on scanned keys | Captures an engine version map at `BEGIN`. If a concurrent transaction commits a write to any key read or modified by this transaction, `COMMIT` aborts with `ErrConflict` (First-Committer-Wins). |

### 5.3 Write Buffering & Commit Protocol
1. **In-Flight Operations:** During an active transaction, all `Set` and `Delete` operations are held in a private session write-buffer.
2. **Query Isolation:** Reads first inspect the local write buffer; if the key is absent, they read from the underlying storage engine, guaranteeing that other concurrent sessions never observe uncommitted mutations (no dirty reads).
3. **Commit Execution:** Upon `COMMIT`, the storage engine acquires its master write mutex, verifies version tags (in Repeatable Read), and flushes the entire write set to the WAL.
4. **Crash Semantics:** Writes are applied sequentially to the WAL under lock. As documented in Phase 5, in the event of a crash midway through multi-key commit execution, a prefix of the records is preserved; single-frame atomic WAL transactions represent a planned enhancement.

---

## 6. Distributed Consensus: The In-Tree Raft Implementation

NebulaDB implements the Raft consensus protocol directly in `internal/raft/` to guarantee linearizable, fault-tolerant replication across node clusters.

```
                      Node 1 (Leader)
                 ┌───────────────────────┐
                 │ Term: 2, Log: [1..5]  │
                 └───────────┬───────────┘
                             │
            AppendEntries    │    AppendEntries
            RPC (Heartbeat)  │    RPC (Heartbeat)
            50ms Interval    │    50ms Interval
                             ▼
     Node 2 (Follower)               Node 3 (Follower)
 ┌───────────────────────┐       ┌───────────────────────┐
 │ Term: 2, Log: [1..5]  │       │ Term: 2, Log: [1..5]  │
 └───────────────────────┘       └───────────────────────┘
```

### 6.1 State Machine Roles
Every node operates in one of three states:
- **Follower:** Listens for incoming `AppendEntries` heartbeats from the leader. If no heartbeat is received before the randomized election timer expires, the node transitions to Candidate.
- **Candidate:** Increments `currentTerm`, votes for itself, and broadcasts `RequestVote` RPCs to all peers.
- **Leader:** Manages log replication, coordinates commits, and sends periodic heartbeats to maintain authority.

### 6.2 Key Protocol Timers & Parameters
- **Heartbeat Interval:** **50 milliseconds** (`Heartbeat = 50 * time.Millisecond`).
- **Election Timeout:** Randomized between **200 milliseconds and 400 milliseconds** (`ElectionMin = 200ms`, `ElectionMax = 400ms`). Randomization prevents split-vote deadlocks during elections.
- **Quorum Requirement:** Commit requires agreement from a strict majority:
  $$\text{Quorum} = \lfloor N/2 \rfloor + 1$$
  For a 3-node cluster, 2 nodes must acknowledge; for a 5-node cluster, 3 nodes must acknowledge.

### 6.3 In-Tree Consensus vs. External Dependencies
- Rather than wrapping an off-the-shelf library like `hashicorp/raft` or using `etcd`, NebulaDB implements election state machines, log replication, index tracking (`commitIndex`, `lastApplied`, `nextIndex`, `matchIndex`), and state persistence in pure Go.
- **State Persistence:** `currentTerm`, `votedFor`, and log entries are durably persisted to `<data>/raft/state.gob` on disk, allowing nodes to recover their voting and log history across crashes.
- **No-Op Leader Assertion:** When a new leader takes office, it immediately appends and commits a blank no-op entry to assert log leadership and commit entries from prior terms (Raft paper §5.4.2).

---

## 7. Horizontal Sharding & Multi-Raft Architecture

To scale beyond a single storage node, NebulaDB implements consistent hash routing (Phase 7) combined with an independent Raft group per shard (Phase 8).

### 7.1 Ketama Consistent Hash Ring (`internal/shard/ring.go`)
- **Virtual Nodes:** Each physical shard is assigned **64 virtual nodes** (`defaultVNodes = 64`) distributed along a $2^{64}-1$ integer ring.
- **Hash Function:** Virtual node points are generated using 64-bit Fowler–Noll–Vo hashing (`FNV-64a`) on the token string `shardID#vnodeIndex`.
- **Key Routing:** When a key is accessed, the router calculates `FNV-64a(key)` and performs a binary search (`sort.Search`) along the sorted virtual node array to locate the first clockwise node.
- **Minimal Key Disruption:** Adding or removing a shard remaps approximately only $\mathbf{1/N}$ of existing keys, avoiding global rehashing.

### 7.2 Full LSM Key Hashing & Scatter-Gather Prefix Scans
- **Routing Decision (ADR-009):** The router hashes the **entire LSM key** (including its table or index prefix). This allows table rows, secondary index entries, and catalog records to share a unified storage engine interface without secondary routing schemes.
- **Scatter-Gather:** Point lookups (`Get`) hash directly to a single shard. Range scans and table scans (`ScanPrefix`) scatter across **all $N$ shards** simultaneously, merge their returned sorted streams, and emit a unified, sorted result set.

### 7.3 Rebalancing Protocol
When shards are added or removed:
1. The engine scans all active keys on existing shards.
2. Evaluates `ring.Lookup(key)`. If the key belongs to a different shard, it writes the key to the new owner (`Set`) and subsequently removes it from the old owner (`Delete`).
3. Executing `Set` before `Delete` ensures that a crash midway through migration produces duplicate keys rather than data loss.

### 7.4 Multi-Raft: Independent Consensus Per Shard
In Phase 8, NebulaDB introduces Multi-Raft:
- **Failure Domain Isolation:** Instead of funneling all database writes through a single global Raft log, **each shard operates its own independent Raft consensus group**.
- **Port Allocation Strategy:** Physical nodes reuse their peer identities (`n1`, `n2`, `n3`), while Raft transports listen on distinct TCP ports calculated as:
  $$\text{Port}_{\text{Raft}} = \text{BasePort} + \text{ShardIndex}$$
  For example, with `--shards 2` and base port `7100`:
  - Shard 0 communicates across `:7100`, `:7200`, `:7300`.
  - Shard 1 communicates across `:7101`, `:7201`, `:7301`.
  - Client KV RPC listens on `:7102` (`BasePort + NumShards`).
- **Resilience:** A network partition or leader failure in Shard 0 has zero operational impact on Shard 1, which continues committing client writes at full speed.

---

## 8. Networking, Client RPC & Chaos Fault Injection

NebulaDB implements a dedicated client networking tier (`internal/cluster/rpc.go`) and a chaos testing infrastructure (`internal/cluster/chaos_test.go`, `cmd/nebulactl`).

### 8.1 Client KV RPC Layer
- **Unified Protocol (ADR-011):** Built on Go's standard `net/rpc` over TCP, sharing serialization patterns with internal Raft communications rather than introducing complex gRPC/Protobuf build overhead.
- **Automatic Forward-to-Leader:** If a client submits a write (`Set` or `Delete`) to a node that is currently a follower for the target key's shard, the follower transparently proxies the RPC to the active shard leader.
- **Backpressure Semaphore:** To prevent server memory exhaustion during traffic spikes, concurrent in-flight RPC handlers are capped at **32 active requests** via a token-bucket semaphore channel.
- **Timeouts & Retries:** Client RPC requests enforce a strict **2-second deadline**. If a leader election is in progress, requests retry automatically upon receiving `ErrNotLeader`.

### 8.2 Chaos Engineering & Fault Injection (`nebulactl`)
NebulaDB includes built-in chaos gates to validate distributed resilience:
- **`nebulactl <addr> isolate`:** Activates an in-memory transport filter (`raft.Gate`) that drops all network RPC packets to and from the specified node, simulating a complete network partition.
- **`nebulactl <addr> heal`:** Restores network connectivity, allowing isolated nodes to rejoin the cluster, catch up on missing log entries, and resume serving traffic.
- **`nebulactl <addr> disk-fail <count>`:** Instructs the local storage engine to fail the next $N$ write operations with `storage.ErrDisk` prior to appending to the WAL, validating that disk write errors do not corrupt the MemTable.
- **`nebulactl <addr> kill-node`:** Issues an administrative termination command over RPC, validating automated cluster leader re-election.

---

## 9. Observability, Metrics & Kubernetes Cloud-Native Deployment

NebulaDB features native Prometheus telemetry and Kubernetes deployment configurations (`internal/metrics/`, `deploy/`).

### 9.1 Zero-Dependency Prometheus Metrics (`/metrics`)
NebulaDB implements a lightweight Prometheus text format generator in `internal/metrics/metrics.go` without importing third-party dependencies:
- **`nebuladb_wal_fsyncs_total` (Counter):** Tracks physical disk sync operations performed by the WAL.
- **`nebuladb_compactions_total` (Counter):** Tracks completed SSTable merge-compaction cycles.
- **`nebuladb_raft_elections_total` (Counter):** Records triggered leader election attempts.
- **`nebuladb_raft_leader_changes_total` (Counter):** Records successful leadership transitions.
- **`nebuladb_ops_total{op="..."}` (Counter):** Total count of `get`, `set`, `delete`, and `scan` operations.
- **`nebuladb_op_duration_seconds{op="..."}` (Histogram):** Latency distribution buckets for storage operations.

### 9.2 Kubernetes StatefulSet Architecture (`deploy/k8s/nebuladb.yaml`)
- **Stateful Pods:** Deployed as a Kubernetes `StatefulSet` with stable network identifiers (`nebuladb-0`, `nebuladb-1`, `nebuladb-2`).
- **Persistent Volume Claims:** Utilizes `volumeClaimTemplates` to attach dedicated persistent block storage mounted at `/data`, ensuring WAL logs and SSTables survive pod restarts or node rescheduling.
- **Headless Service:** Configured with `clusterIP: None` to provide stable internal DNS SRV records for peer discovery.
- **Liveness & Readiness Probes:**
  - `GET /livez`: Returns HTTP 200 as long as the process is running.
  - `GET /readyz`: Returns HTTP 200 when the storage engine and Raft groups are initialized; returns HTTP 503 during graceful shutdown.
- **Graceful Shutdown:** Configured with a 30-second `terminationGracePeriodSeconds`. Upon receiving SIGTERM, the process marks itself unready, closes active listeners, and executes `Engine.Close()` (flushing dirty WAL pages to disk) before process exit.

---

## 10. Analytics Layer, Star Schema & Python Data Engineering

Phase 12 bridges operational database storage with downstream analytics workloads (`analytics/`).

### 10.1 Python ETL Pipeline (`analytics/etl.py`)
- **Cleansing & Ingestion:** Reads dirty raw order datasets (`raw_orders.csv`), cleanses corrupted records (stripping whitespace, lowercasing emails, validating positive integer amounts), and normalizes customer city names.
- **Star Schema Normalization:** Splits raw records into a normalized dimensional star schema:
  - `dim_customer (id INT, email TEXT, city TEXT)`
  - `fact_order (id INT, customer_id INT, amount INT)`
- **SQL Pipeline Generation:** Automatically generates executable NebulaDB DDL and INSERT statements, followed by analytical aggregation queries.

### 10.2 Analytical SQL Capabilities
NebulaDB natively executes relational analytical queries:
```sql
SELECT city, COUNT(*), SUM(amount)
FROM fact_order
INNER JOIN dim_customer ON fact_order.customer_id = dim_customer.id
GROUP BY city
ORDER BY city ASC;
```

### 10.3 Downstream Big Data & BI Integrations
- **PySpark Integration (`analytics/spark_job.py`):** Provides a dual-mode aggregation job. If PySpark is present, it runs via `SparkSession` with distributed `.groupBy("city").agg(...)`; if absent, it falls back to standard-library aggregation, ensuring local testability.
- **BI Connectors:** Emits normalized extracts compatible with Tableau (`orders_star.tds`), Power BI (`orders.pbids`), and standard CSV/TSV formats.
- **Natural Language to SQL (`analytics/nl_sql.py`):** Demonstrates rule-based semantic translation of English questions ("sales by city", "how many orders") into valid NebulaDB SQL statements.

---

## 11. The Master Metric & Benchmark Ledger (All Verified Numbers)

These figures represent verified measurements recorded from benchmark runs on a 12th Gen Intel Core i7-12650H running Windows 11 (`docs/benchmarks.md`):

### 11.1 Benchmark Results (`cmd/bench -n 5000`)

| Operation | Durability (`SyncMode`) | Throughput (QPS) | Mean Latency | p99 Latency (Sampled) | Key Takeaway |
|---|:---:|:---:|:---:|:---:|---|
| **`Set`** | **`SyncAlways`** | **14,749 QPS** | **67,803 ns** (~67.8 µs) | **999,700 ns** (~1.0 ms) | Physical disk `fsync` executed after every record |
| **`Get`** | *(After Sync)* | **1,515,106 QPS** | **660 ns** (~0.66 µs) | **0 ns** (< timer resolution) | Sub-microsecond reads from skip list MemTable |
| **`Set`** | **`SyncNone`** | **330,570 QPS** | **3,025 ns** (~3.0 µs) | **0 ns** (< timer resolution) | OS page cache buffer; no immediate disk sync |
| **`Get`** | *(After NoSync)* | **1,761,121 QPS** | **567 ns** (~0.57 µs) | **0 ns** (< timer resolution) | Peak read throughput exceeding 1.7M QPS |

### 11.2 Performance & System Ratios
- **The Physical Cost of Durability ($\mathbf{f_{\text{sync}}}$):**
  - Moving from `SyncAlways` (14,749 QPS) to `SyncNone` (330,570 QPS) yields a **22.41x throughput speedup** (+2,141%).
  - The mean write latency drops from **67.8 µs down to 3.0 µs**—a **95.54% reduction in write latency**—quantifying the overhead of physical disk controller flushes.
- **Read-to-Write Ratio:** In-memory MemTable reads (1.51M QPS) are **102.7x faster** than durable disk-synced writes (14.7k QPS).

### 11.3 Architectural Constants & Configuration Values
- **`13 bytes`:** Fixed WAL record header size (`crc32: 4B + type: 1B + klen: 4B + vlen: 4B`).
- **`24 bytes`:** Fixed SSTable footer size (`index_off: 8B + bloom_off: 8B + magic: 4B + crc32: 4B`).
- **`0x4C535431`:** SSTable magic byte constant (ASCII `"LST1"`).
- **`4,096 bytes (4 KiB)`:** SSTable index restart interval (`restartEvery = 4096`).
- **`1% (p = 0.01)`:** Design false positive probability for the SSTable Bloom filter.
- **`1 MiB (1,048,576 bytes)`:** Default MemTable memory flush limit (`MemtableBytes = 1 << 20`).
- **`4 SSTables`:** Compaction threshold trigger (`CompactN = 4`).
- **`64 Virtual Nodes`:** Virtual nodes assigned per physical shard in the Ketama hash ring.
- **`50 milliseconds`:** Raft heartbeat interval (`Heartbeat = 50ms`).
- **`200ms – 400ms`:** Randomized Raft election timeout range (`ElectionMin` to `ElectionMax`).
- **`32 requests`:** Maximum concurrent in-flight client RPC backpressure semaphore.
- **`2 seconds`:** Default client KV RPC timeout deadline.
- **`30 seconds`:** Kubernetes pod `terminationGracePeriodSeconds`.

---

## 12. "Why This Instead of That?" — Critical Architectural Trade-Offs (ADRs)

Senior database engineering interviews focus heavily on design choices and trade-offs. Here is the rationale behind NebulaDB's architecture:

### 12.1 Why Go Instead of C++ or Rust? (ADR-001)
- **Concurrency & Goroutines:** Go provides lightweight runtime-managed goroutines and channels, simplifying the development of concurrent Raft state machines, background compactions, and timer loops.
- **Built-in Race Detection:** The `-race` flag in Go's toolchain makes identifying concurrency bugs and memory race conditions during multi-node chaos testing fast and reliable.
- **Iteration Speed vs. System Purity:** While C++ and Rust offer fine-grained memory management without garbage collection pauses, Go allows rapid iteration while still providing low-level syscalls (`os.File.Sync`), binary serialization, and memory pointers.

### 12.2 Why LSM-Tree Instead of a B+ Tree for Core Storage? (ADR-002)
- **Sequential Disk I/O:** Traditional B+ trees rely on random in-place page writes, leading to high disk write amplification and random I/O bottlenecks. LSM trees convert all writes into sequential append operations (WAL append, MemTable insert), maximizing physical SSD/NVMe throughput.
- **Natural Alignment with Raft:** The append-only nature of WAL and SSTables mirrors Raft's append-only replicated log, simplifying state machine application and snapshot recovery.
- **Trade-off Acknowledged:** LSM trees pay a read amplification cost (querying MemTable and multiple SSTables). NebulaDB mitigates this using in-memory Bloom filters (1% FPR) and 4 KiB block indexes.

### 12.3 Why an In-Tree Raft Engine Instead of Importing etcd/Raft? (ADR-003)
- **Architectural Mastery:** Importing a pre-built library obscures the mechanics of split-brain prevention, term changes, and log matching. Implementing Raft from scratch ensures deep ownership of the consensus engine.
- **Tailored State Machine Coupling:** An in-tree implementation enables clean integration with the local LSM engine, custom RPC gates for chaos injection, and Multi-Raft port offsetting.

### 12.4 Why WAL-Before-MemTable? (ADR-004)
- Updating the in-memory MemTable *before* disk synchronization creates a vulnerability window: if the process crashes immediately after updating memory, a client that received a success confirmation loses its committed write upon restart.
- Appending and fsyncing the WAL first guarantees that every write acknowledged to a client is durably recoverable from disk.

### 12.5 Why Strict CRC Checksums Over Silent Skipping? (ADR-005)
- Many storage engines attempt to salvage corrupted logs by silently skipping corrupted records. NebulaDB fails immediately upon encountering a CRC mismatch during recovery. Silent skips hide disk controller defects, bit flips, or underlying storage bugs, leading to silent data divergence across distributed replicas.

### 12.6 Why net/rpc Over TCP Instead of gRPC/Protobuf? (ADR-011)
- **Unified RPC Stack:** Raft consensus already communicates via TCP RPC. Introducing gRPC for client operations would require maintaining two distinct networking stacks, two port listeners, and an external Protobuf code compilation pipeline without adding any distributed consistency guarantees.
- `net/rpc` delivers clean Go-native serialization with low CPU overhead.

### 12.7 Why Hash the Full LSM Key in Sharding? (ADR-009)
- Sharding strictly by SQL primary key requires secondary index keys and catalog schemas to implement a secondary routing scheme, complicating multi-index updates.
- By hashing the **entire LSM key** (e.g., `p<table>\x00<pk>` or `i<table>\x00<idx>\x00<sk><pk>`), the router treats all keys uniformly as a pure `storage.KV` interface.
- **Trade-off Acknowledged:** Table scans and prefix scans cannot target a single shard; they must execute scatter-gather queries across all shards and merge results.

### 12.8 Why Independent Raft Groups Per Shard (Multi-Raft)? (ADR-010)
- Running a single global Raft log across an entire multi-shard node forces all database operations through a single leader and one linear `commitIndex`, creating a throughput bottleneck.
- Multi-Raft isolates consensus per shard: Shard 0 can elect a new leader or recover from a network partition while Shard 1 continues committing writes at full capacity.

### 12.9 Why Nested-Loop Join in the SQL Engine?
- For an educational and modular engine, a nested-loop join provides a correct, readable implementation for arbitrary equi-joins (`ON tableA.id = tableB.customer_id`).
- While hash joins and sort-merge joins offer $O(N + M)$ performance for large datasets, nested-loop joins establish correctness before introducing cost-based optimizer complexity.

---

## 13. Future Systems Roadmap & Scalability Enhancements

While Phases 1 through 14 are fully implemented and verified, production enterprise deployment introduces several natural extension points:

1. **Two-Phase Commit (2PC) / Percolator Protocol:** Currently, multi-shard transactions apply mutations sequentially without cross-shard atomicity. Implementing a 2PC coordinator or Google Percolator-style snapshot isolation would provide atomic cross-shard transactions.
2. **Raft Log Compaction & Snapshotting:** Implementing the `InstallSnapshot` RPC to truncate historical Raft logs and send compacted SSTable snapshots to lagging follower nodes.
3. **Cost-Based Query Optimizer (CBO):** Replacing heuristic query plans with statistical table cardinality estimation, column histograms, and dynamic join-order selection (Hash Join vs. Nested-Loop).
4. **Block Cache (LRU / 2Q):** Adding an uncompressed in-memory block cache for frequently accessed 4 KiB SSTable blocks to reduce NVMe read IOPS.
5. **Dynamic Cluster Membership Changes:** Transitioning from static configuration peer lists to dynamic cluster reconfiguration via Raft joint consensus (Raft §6).
6. **Vector Search Indexing (HNSW):** Introducing vector embedding data types and Hierarchical Navigable Small World (HNSW) graph indexing over LSM storage for AI/RAG search workloads.

---

## 14. Master Technical Interview Preparation: Questions & Senior Model Answers

### Q1: "Walk me through the write path of NebulaDB. What happens from the moment a write is received to when it is considered durable?"
> **Model Answer:**  
> "In NebulaDB, the write path depends on whether we are operating in standalone, sharded, or clustered mode:  
> 1. **Routing & RPC:** The client submits a `Set(key, value)` to a node via our TCP `net/rpc` interface. If the recipient node is not the leader for the key's shard, it transparently forwards the request to the active shard leader. An inflight semaphore limits concurrent handlers to 32 to prevent memory exhaustion.  
> 2. **Consistent Hashing:** The key's target shard is determined by hashing the full LSM key onto our Ketama consistent hash ring (64 virtual nodes per physical shard).  
> 3. **Consensus Replication (Raft):** The shard leader appends the mutation to its in-tree Raft log and broadcasts an `AppendEntries` RPC to its peers. The leader waits until a strict majority of nodes have persisted the entry. Once majority agreement is reached, the leader advances its `commitIndex`.  
> 4. **Local Engine Application:** The committed entry is handed to the local LSM engine:  
>    - It is serialized into a binary WAL record with a 13-byte header and an IEEE CRC32 checksum.  
>    - The record is appended to `wal-000001.log`. If `SyncMode` is `SyncAlways`, `os.File.Sync()` is called immediately to flush physical disk buffers.  
>    - Only after `fsync` returns success is the key inserted into the active SkipList MemTable.  
> 5. **Success Response:** Success is returned to the client. If the MemTable exceeds its 1 MiB threshold, a background flush converts it into an immutable SSTable with a Bloom filter and a 4 KiB block index."

---

### Q2: "How does NebulaDB prevent data loss or silent corruption during crash recovery?"
> **Model Answer:**  
> "Crash recovery is handled through a combination of WAL framing, strict checksumming, and idempotent replay:  
> 1. **WAL Framing:** Every WAL record starts with a 4-byte CRC32, a 1-byte record type, and 4-byte length prefixes for both key and value.  
> 2. **Torn Write Handling:** If the operating system crashed mid-write, the final record in the log file will be truncated. During recovery, NebulaDB scans the WAL sequentially. If a record header is incomplete or the file ends before reading the full declared payload length, the engine automatically truncates the log back to the last valid byte offset, discarding the incomplete mutation.  
> 3. **Strict CRC Validation:** For every intact record, the engine recalculates the IEEE CRC32 across the payload. If the calculated checksum does not match the header checksum (indicating a bit flip or bad disk sector), the engine aborts recovery with an error rather than silently skipping the record. Silent skipping can mask storage corruption and cause state divergence.  
> 4. **Idempotence:** Replaying the log applies `Put` and `Delete` operations to a fresh MemTable. Because these operations are idempotent, replaying the same log multiple times always reconstructs the identical database state."

---

### Q3: "Explain your LSM read path. How do you mitigate read amplification?"
> **Model Answer:**  
> "Because LSM trees do not perform in-place updates, a key could exist in the active MemTable or across multiple on-disk SSTables. To locate a key:  
> 1. We first check the in-memory **SkipList MemTable**. If the key is present (or an explicit tombstone is found), we return immediately.  
> 2. If missed, we consult the **`MANIFEST`**, which tracks live SSTables ordered from newest to oldest. We inspect SSTables in reverse chronological order.  
> 3. **Bloom Filter Pruning:** Every SSTable has an in-memory Bloom filter sized for a 1% false positive rate. If the filter returns false, we know with mathematical certainty that the key is not in that SSTable, completely avoiding disk I/O.  
> 4. **Two-Level Block Indexing:** If the Bloom filter matches, we read the SSTable's index block. Restart keys recorded every 4 KiB allow us to binary search for the exact 4 KiB block containing the key, rather than scanning the entire file.  
> 5. **Size-Tiered Compaction:** To keep the number of SSTables small, whenever the count reaches 4, background compaction executes an $N$-way merge-sort, collapsing historical files into a single sorted SSTable and removing dead tombstones."

---

### Q4: "How does your in-tree Raft consensus engine handle network partitions and split-brain scenarios?"
> **Model Answer:**  
> "NebulaDB enforces strict majority quorums for all leader elections and log commits:  
> 1. **Majority Requirement:** For an $N$-node cluster, any election or log commit requires an agreement from $\lfloor N/2 \rfloor + 1$ nodes. In a 3-node cluster, quorum is 2.  
> 2. **Symmetric Partition Scenario:** Suppose a 3-node cluster `{n1, n2, n3}` has leader `n1`. If a network partition isolates `n1` on one side and `{n2, n3}` on the other:  
>    - Node `n1` can no longer receive heartbeats from or replicate to a majority. Any client write proposed to `n1` will block or fail because it cannot achieve the required 2-node quorum.  
>    - Nodes `n2` and `n3` will miss heartbeats. Their randomized election timers (200ms–400ms) will expire, triggering an election. One of them (e.g., `n2`) will collect votes from both `n2` and `n3`, establish quorum, increment the term, and become the new leader.  
>    - Clients communicating with `{n2, n3}` can successfully commit writes.  
> 3. **Healing the Partition:** When the partition heals, `n1` receives an `AppendEntries` RPC from `n2` with a higher term number. `n1` immediately steps down to follower status, updates its term, and truncates any uncommitted log entries that conflict with `n2`'s log, preventing split-brain corruption. We validated this behavior using our chaos injection harness (`nebulactl isolate/heal`)."

---

### Q5: "How does Multi-Raft in NebulaDB differ from a standard single-group Raft database?"
> **Model Answer:**  
> "In a single-group Raft database, every write in the entire system passes through a single Raft log and a single leader. Even if data is partitioned across multiple disks, the single leader's network bandwidth and serial `commitIndex` become an architectural bottleneck.  
> NebulaDB implements **Multi-Raft**:  
> 1. Each shard is an entirely independent Raft consensus group with its own leader, followers, term counter, and commit index.  
> 2. Physical nodes run multiple shard state machines simultaneously. We offset peer network ports by shard index (`basePort + shardIndex`), allowing shard groups to communicate over isolated TCP sockets.  
> 3. If the leader for Shard 0 experiences a hardware fault or network partition, Shard 0 initiates an election. Meanwhile, Shard 1's leader continues replicating and committing client writes without interruption. This isolates failure domains and scales write throughput linearly with the number of shards."

---

### Q6: "Why did you implement an order-preserving byte encoding for secondary indexes?"
> **Model Answer:**  
> "In our architecture, secondary indexes are stored as native keys in the same LSM tree as table rows (`i<table_name>\x00<index_name>\x00<encoded_sk><pk>`).  
> Because the LSM tree orders keys using raw lexicographical byte comparison (`bytes.Compare`), naive string or integer serialization breaks range queries. For example, if we simply concatenated strings containing null bytes (`0x00`), the delimiter between the secondary key and the primary key would be ambiguous.  
> We implemented an order-preserving byte-stuffing algorithm:  
> - Any internal null byte `0x00` in the secondary key value is escaped to `0x00 0xFF`.  
> - The end of the secondary key is marked with the terminator `0x00 0x01`.  
> - Because `0x01 < 0xFF`, standard byte comparison guarantees that any prefix matching the search value sorts ahead of longer keys, while maintaining correct alphabetical and numerical ordering across the LSM index. This allows SQL range scans (`WHERE age >= 21 AND age <= 65`) to execute clean prefix scans over the LSM storage engine."

---

### Q7: "How did you design the benchmarking harness, and what did the numbers reveal about the physical cost of durability?"
> **Model Answer:**  
> "We built `cmd/bench` as a standalone CLI tool measuring sequential `Set` and `Get` operations across 5,000 keys with 32-byte values, comparing our two durability modes:  
> - Under **`SyncAlways`**, every write calls `fsync` to force the physical drive controller to flush its cache. We measured **14,749 QPS** with a mean latency of **67.8 microseconds** and a p99 of **999.7 microseconds**.  
> - Under **`SyncNone`**, writes append to the WAL and enter the OS page cache without immediate `fsync`. Here, throughput reached **330,570 QPS** with a mean latency of **3.0 microseconds**.  
> This comparison demonstrates that physical disk synchronization accounts for a **22.4x throughput penalty** and **95.5% of total write latency**. It proves why modern production databases implement group commit (batching multiple concurrent writes into a single `fsync`) to achieve high durability without sacrificing throughput."

---

### Q8: "How does the Kubernetes deployment guarantee data persistence across pod restarts?"
> **Model Answer:**  
> "A standard Kubernetes Deployment is stateless; if a pod crashes or is rescheduled, its local filesystem is destroyed.  
> In NebulaDB:  
> 1. We deploy using a **`StatefulSet`** combined with **`volumeClaimTemplates`**. Each replica (`nebuladb-0`, `nebuladb-1`) receives an independent, dedicated persistent volume (PVC) mounted to `/data`.  
> 2. When a pod is restarted or upgraded, Kubernetes reattaches the identical physical block volume to the new pod instance, preserving the WAL logs, SSTables, and `MANIFEST`.  
> 3. We use a **Headless Service** (`clusterIP: None`) so that each node has a stable, predictable DNS name (`nebuladb-0.nebuladb.default.svc.cluster.local`), ensuring that peer addresses in the Raft configuration remain constant across pod restarts.  
> 4. We implement custom HTTP `/livez` and `/readyz` probes. During a planned shutdown, the container intercepts `SIGTERM`, unregisters from readiness, and executes `Engine.Close()`, ensuring all pending WAL buffers are cleanly synced to disk before termination."

---

## 15. Quick Interview Talking Points & High-Impact Summary

When presenting NebulaDB in an interview, anchor your discussion on these core accomplishments:

1. **True Systems Implementation:** Built storage, consensus, and SQL from scratch in Go using standard library primitives—not a wrapper around SQLite, RocksDB, or etcd.
2. **Crash-Safe Storage Engine:** Append-only WAL with 13-byte binary headers, IEEE CRC32 checksums, and torn-write auto-truncation, feeding a concurrent SkipList MemTable.
3. **LSM Architecture:** Immutable SSTables with Bloom filters (1% FPR), 4 KiB restart block indexing, size-tiered compaction ($N \ge 4$), and a stateful `MANIFEST`.
4. **In-Tree Raft Consensus:** Built an election and replication state machine with randomized timeouts (200–400ms), 50ms heartbeats, persistent disk logs (`state.gob`), and majority quorum guarantees.
5. **Horizontal Multi-Raft Sharding:** Ketama consistent hashing (64 vnodes per shard) routing full LSM keys, with scatter-gather prefix scans and independent Raft consensus groups per shard on offset ports.
6. **Robust Concurrency & Transactions:** Read Committed and Repeatable Read isolation levels with optimistic conflict aborts (`ErrConflict`).
7. **Production Observability & Cloud-Native Deployment:** Zero-dependency Prometheus `/metrics`, Kubernetes StatefulSet manifests with PVC volume claim templates, and graceful SIGTERM shutdown.
8. **Empirical Benchmarks:** Measured **14,749 QPS** under durable `SyncAlways` (67.8 µs latency) and **330,570 QPS** under `SyncNone` (3.0 µs latency), proving the physical cost of storage durability.
