# NebulaDB

A distributed, fault-tolerant SQL database engine written in Go — **from scratch**.

Not a CRUD app. Not a PostgreSQL wrapper. This project implements storage, recovery, consensus, and a focused SQL subset as real internals.

```
                     SQL Client
                         │
                         ▼
                 ┌──────────────┐
                 │ Query Router │
                 └──────┬───────┘
                        │
             ┌──────────┼──────────┐
             ▼          ▼          ▼
          Node 1     Node 2     Node 3
          Leader     Follower   Follower
             │          │          │
             └──────────┼──────────┘
                        │
                  Raft Consensus
                        │
              ┌─────────┴─────────┐
              ▼                   ▼
          WAL + MemTable       SSTables
              │                   │
              └─────────┬─────────┘
                        ▼
                  Disk Storage
```

## Current status

**Phase 5 — Transactions** is current: `BEGIN` / `COMMIT` / `ROLLBACK` with Read Committed (default) and Repeatable Read.

| Capability | Status |
|------------|--------|
| KV, WAL, LSM, indexes | Implemented |
| SQL subset | Implemented |
| Transactions + isolation | Implemented |
| Raft cluster | Phase 6 |

Do not claim distributed SQL, Raft, or Kubernetes until those phases exist and are tested.

## Quick start

Requires Go 1.22+.

```bash
go test ./...
go test -race ./...

go run ./cmd/nebuladb --data ./data
```

Interactive commands:

```
CREATE TABLE users (id INT, name TEXT, age INT);
BEGIN;
INSERT INTO users VALUES (1, 'Jayant', 20);
COMMIT;
SELECT * FROM users;
```

Restart with the same `--data` directory after a crash. Mutations are durable once the WAL append has been synced.

## Documentation

| Doc | Purpose |
|-----|---------|
| [docs/roadmap.md](docs/roadmap.md) | Phased plan and acceptance criteria |
| [docs/architecture.md](docs/architecture.md) | Target architecture vs what exists today |
| [docs/storage.md](docs/storage.md) | WAL, MemTable, durability, recovery |
| [docs/consistency.md](docs/consistency.md) | Intended consistency model |
| [docs/decisions.md](docs/decisions.md) | Architecture Decision Records |

## Principles

1. **Implement internals.** Storage, recovery, and consensus are first-class code.
2. **Prove, don't advertise.** Throughput and p99 come from benchmarks. Never invent numbers.
3. **Each phase is independently demoable.** A WAL-backed KV store is already interview-grade if you can explain `fsync` and replay.
4. **Correctness before distribution.** A wrong single-node engine plus Raft is still a wrong cluster.

## License

MIT — see [LICENSE](LICENSE).
