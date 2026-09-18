# Phase 2 — LSM Tree

Commit **one step at a time**. Do not squash these into a single “add LSM” commit.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 2.0 | On-disk SST / MANIFEST / flush design | `docs: specify LSM SSTable and flush protocol` |
| 2.1 | Bloom filter | `feat(storage): add Bloom filter` |
| 2.2 | SSTable writer/reader | `feat(storage): add SSTable files` |
| 2.3 | MemTable flush + WAL rotate + Get stack | `feat(storage): flush MemTable to SSTables` |
| 2.4 | Size-tiered compaction + tombstone GC | `feat(storage): compact SSTables` |
| 2.5 | README / storage.md status | this file |

## Definition of done

- [x] MemTable flushes to an immutable SSTable when it exceeds a size threshold
- [x] `Get` checks MemTable, then SSTables newest-first (Bloom skip)
- [x] Restart loads MANIFEST SSTables, then replays only the **post-flush** WAL
- [x] Compaction merges SSTables and drops tombstones when no older version can exist
- [x] Tests cover flush, recovery after flush, and compaction last-write-wins

## Flush protocol (crash order)

1. Write `sst/NNNNNN.sst` and `fsync`
2. Persist MANIFEST listing that file (SST is now recoverable)
3. Truncate/rotate WAL

Crash after (1) only: orphan SST, WAL still complete → safe.  
Crash after (2) before (3): SST + WAL overlap → Get still last-write-wins.  
Never rotate the WAL before the SST is in MANIFEST.
