# Phase 1 — Storage Engine

Durable single-node key-value store: WAL + skip-list MemTable + crash recovery.

## Steps

| Step | Deliverable | Commit |
|------|-------------|--------|
| 1.0 | Architecture, roadmap, ADRs | `docs: add NebulaDB architecture...` |
| 1.1 | Go module, Makefile, CLI stubs | `chore: scaffold Go module...` |
| 1.2 | WAL: framed records, CRC32, fsync, replay | `feat(storage): add CRC-framed WAL...` |
| 1.3 | Skip-list MemTable + tombstones | `feat(storage): add skip-list MemTable...` |
| 1.4 | Engine: WAL-then-MemTable, Open/Close | `feat(storage): wire Engine...` |
| 1.5 | REPL + recovery tests | this step |

## Definition of done

- [x] `Set`/`Get`/`Delete` on byte keys/values
- [x] Every mutation appended to WAL before becoming visible
- [x] Configurable `fsync` (`SyncAlways` / `SyncNone`)
- [x] Process restart reconstructs state from WAL
- [x] Truncated last record on crash does not corrupt the log
- [x] Concurrent writers covered by tests (`go test ./...`; use `-race` when CGO/gcc is available)

**Out of scope:** SSTables, SQL, network, Raft.

## Demo

```bash
go test ./...
go run ./cmd/nebuladb --data ./data
```
