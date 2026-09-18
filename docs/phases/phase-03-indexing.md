# Phase 3 — Indexing

SQL `CREATE INDEX` waits for Phase 4. This phase ships the **index storage layer** so the planner has something real to call.

Commit **one step at a time**.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 3.0 | Index key format, catalog, maintenance rules | `docs: specify secondary indexes and B+ tree` |
| 3.1 | Length-prefixed row codec | `feat(row): encode tuples for indexed fields` |
| 3.2 | In-memory B+ tree (insert, split, search, range) | `feat(index): add B+ tree` |
| 3.3 | Prefix scan on the LSM engine | `feat(storage): prefix scan for secondary keys` |
| 3.4 | LSM secondary indexes + catalog + backfill | `feat(index): LSM secondary indexes` |
| 3.5 | REPL: `row` / `idxcreate` / `idxfind` | `feat(cmd): indexed row commands` |

## Definition of done

- Rows are tuples: primary key + ordered fields
- `CreateIndex(name, field)` persists a catalog entry and **backfills** existing rows
- `Put`/`Delete` maintain secondary keys (old SK removed on update)
- `Find` / `Range` return primary keys in SK order
- Indexes survive restart (they live in the same WAL/LSM as primary data)
- B+ tree splits are unit-tested (the structure SQL indexes will use later for in-memory / dedicated files)

## Key layout (LSM)

```
p | pk                         → encoded row
i | idx | 0x00 | enc(sk) | 0x00 | pk   → empty
x | catalog                    → index metadata
```

`enc(sk)` is order-preserving (null-byte escaped) so `idxrange` is a prefix/range scan, not a filter over the whole table.

## Maintenance

On `PutRow(pk, fields)`:

1. Read previous row (if any)
2. Delete previous secondary keys
3. WAL-backed `Set` of the primary row
4. `Set` each secondary key

Never leave a secondary key pointing at a missing primary after a successful delete.

## B+ tree (this phase)

In-memory, order ≥ 3, leaf sibling pointers for range scans. Disk paging of B+ trees can replace this in a later pass; LSM indexes are the crash-safe default.
