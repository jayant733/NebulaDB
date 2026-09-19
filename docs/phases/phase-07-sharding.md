# Phase 7 — Sharding

A **query router** places each LSM key on one of N local shards using a **consistent hash ring**. Each shard is its own `storage.Engine` (its own WAL and SSTables). Raft-per-shard is Phase 8.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 7.0 | Ring, routing, rebalance notes | `docs: specify sharding` |
| 7.1 | Consistent hash ring | `feat(shard): consistent hash ring` |
| 7.2 | Router KV (`Get`/`Set`/`Delete`/`ScanPrefix`) | `feat(shard): route keys to shards` |
| 7.3 | Rebalance after membership change | `feat(shard): rebalance keys` |
| 7.4 | `--shards` on `nebuladb` | `feat(cmd): local shards` |
| 7.5 | Status docs | `docs: Phase 7 complete` |

## Definition of done

- [x] Same key always maps to the same shard while the ring is unchanged
- [x] `Set`/`Get`/`Delete` go to that shard only
- [x] `ScanPrefix` visits every shard and returns a merged, sorted view
- [x] Adding a shard and `Rebalance` moves only keys whose owner changed; no key is lost
- [x] SQL and the REPL work on the router (scatter-gather scans)
- [x] `--shards` and `--peers` are not combined (replicated shards are Phase 8)

## Routing key

The **full LSM key** is hashed (primary row, secondary index entry, catalog, SQL schema). Prefix scans cannot predict owners, so they **scatter-gather**.

This is not hash-by-table and not hash-by-SQL-PK-only. Secondary-index keys for one row may live on a different shard than the primary row.

## Ring

Ketama-style ring: each physical shard owns `V` virtual nodes (default 64). Virtual node hash is FNV-1a of `id # vnodeIndex`. Lookup hashes the key the same way and walks clockwise to the first vnode.

Adding a shard remaps about `1/N` of keys, not all of them. Jump hash is not used because it assumes dense `0..N-1` IDs and remaps on any N change without a stored ring.

## Rebalance

After the ring gains or loses a shard:

1. For each existing shard, scan all live keys.
2. If `Lookup(key)` is not this shard, `Set` on the new owner then `Delete` on the old.

`Set` before `Delete` so a crash during migrate does not drop the key (duplicate until the next rebalance is acceptable).

Phase 7 does **not** live-migrate while serving a production cluster over the network. Membership is in-process.

## Transactions

`BEGIN`/`COMMIT` still buffer then apply per key through the router. That is **not** atomic across shards: a crash mid-`COMMIT` can persist a prefix of keys, same as Phase 5 on one WAL, but now on several disks. There is no 2PC. Do not claim distributed transactions.

## Process layout

`--shards 3 --data ./data`:

```
data/shard-0/   # Engine
data/shard-1/
data/shard-2/
```

`--shards 1` (default) keeps the Phase 1–6 layout: files directly under `--data`.

```
go run ./cmd/nebuladb --data ./data --shards 3
```
