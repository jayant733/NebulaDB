# Phase 1 execution checklist

Use this file while implementing. Check items only when tests prove them.

## Step 1.0 — Plan (docs)

- [ ] README, roadmap, architecture, storage, consistency, ADRs

## Step 1.1 — Scaffold

- [ ] `go.mod`, Makefile, `cmd/nebuladb` stub

## Step 1.2 — WAL

- [ ] Encode/decode records
- [ ] Append + optional Sync
- [ ] Replay
- [ ] Torn-write truncate
- [ ] CRC failure

## Step 1.3 — MemTable

- [ ] Put / Get / Delete (tombstone)
- [ ] Ordered scan
- [ ] Concurrent access

## Step 1.4 — Engine

- [ ] Open recovers WAL
- [ ] Set/Delete log then apply
- [ ] Close flushes/syncs

## Step 1.5 — REPL + recovery demo

- [ ] Interactive CLI
- [ ] Restart sees data
