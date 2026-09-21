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

**Phase 12 — Analytics** is current: inner join, `GROUP BY` aggregates, and a Python ETL that exports CSV for Tableau / Power BI / Qlik.

| Capability | Status |
|------------|--------|
| KV, WAL, LSM, indexes, Raft | Implemented |
| SQL + JOIN + GROUP BY | Implemented (no windows/CTEs/triggers) |
| Python ETL + BI CSV export | Implemented |
| `/metrics` + Grafana JSON | Implemented |
| Kubernetes | Phase 13 |

Do not claim Tableau, Spark, Databricks, or Snowflake as products we built. See [docs/skills.md](docs/skills.md).

## Quick start

Requires Go 1.22+.

```bash
go test ./...
go test -race ./...

python analytics/etl.py
go run ./cmd/nebuladb --data ./data --metrics 127.0.0.1:9100
```

Three-node cluster (three terminals; write on the leader):

```bash
go run ./cmd/nebuladb --data ./d1 --id n1 --peers n1=127.0.0.1:7001,n2=127.0.0.1:7002,n3=127.0.0.1:7003
go run ./cmd/nebuladb --data ./d2 --id n2 --peers n1=127.0.0.1:7001,n2=127.0.0.1:7002,n3=127.0.0.1:7003
go run ./cmd/nebuladb --data ./d3 --id n3 --peers n1=127.0.0.1:7001,n2=127.0.0.1:7002,n3=127.0.0.1:7003
```

Local shards (one process, three engines):

```bash
go run ./cmd/nebuladb --data ./data --shards 3
```

Two shards, three nodes (shard 1 listens on base port + 1):

```bash
go run ./cmd/nebuladb --data ./d1 --id n1 --shards 2 --peers n1=127.0.0.1:7100,n2=127.0.0.1:7200,n3=127.0.0.1:7300
go run ./cmd/nebuladb --data ./d2 --id n2 --shards 2 --peers n1=127.0.0.1:7100,n2=127.0.0.1:7200,n3=127.0.0.1:7300
go run ./cmd/nebuladb --data ./d3 --id n3 --shards 2 --peers n1=127.0.0.1:7100,n2=127.0.0.1:7200,n3=127.0.0.1:7300
```

Client/admin RPC for n1 is `127.0.0.1:7102` (base + shard count):

```bash
go run ./cmd/nebulactl 127.0.0.1:7102 isolate
go run ./cmd/nebulactl 127.0.0.1:7102 heal
go run ./cmd/nebulactl 127.0.0.1:7102 disk-fail 1
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
| [docs/skills.md](docs/skills.md) | How analytics/SQL/Python skills map to this repo |
| [analytics/README.md](analytics/README.md) | ETL + BI export |

## Principles

1. **Implement internals.** Storage, recovery, and consensus are first-class code.
2. **Prove, don't advertise.** Throughput and p99 come from benchmarks. Never invent numbers.
3. **Each phase is independently demoable.** A WAL-backed KV store is already interview-grade if you can explain `fsync` and replay.
4. **Correctness before distribution.** A wrong single-node engine plus Raft is still a wrong cluster.

## License

MIT — see [LICENSE](LICENSE).
