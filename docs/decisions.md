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
