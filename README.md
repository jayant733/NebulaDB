# NebulaDB

A distributed, fault-tolerant SQL database engine written in Go — **from scratch**.

Not a CRUD app. Not a PostgreSQL wrapper. This project implements storage, recovery, consensus, a focused SQL subset, a Python analytics layer, Kubernetes manifests, and **measured** KV microbenchmarks.

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

**Phases 1–14 of the in-repo plan are implemented.** Throughput belongs in [docs/benchmarks.md](docs/benchmarks.md) only.

| Capability | Status |
|------------|--------|
| KV, WAL, LSM, indexes, Raft, shards | Implemented |
| SQL + JOIN + GROUP BY | Implemented (no windows/CTEs/triggers) |
| Python ETL + BI/Spark/Snowflake recipes | Implemented (tools not embedded) |
| `/metrics` `/livez` `/readyz` + Grafana JSON | Implemented |
| Kubernetes StatefulSet + PVC | Manifests + probes; not a live GKE operator |
| Benchmarks | Measured; see [docs/benchmarks.md](docs/benchmarks.md) |

Do not claim Tableau, Spark, Databricks, or Snowflake as products we built. See [docs/skills.md](docs/skills.md).

## Quick start

Requires Go 1.22+.

```bash
go test ./...
go run ./cmd/bench -n 5000
python analytics/pipeline.py --once
go run ./cmd/nebuladb --data ./data --http 127.0.0.1:8080
```

Container / Kubernetes (image must be loaded into the cluster; CI does not run `kind`):

```bash
docker build -t nebuladb:local .
kubectl apply -f deploy/k8s/nebuladb.yaml
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

Restart with the same `--data` directory after a crash. Mutations are durable once the WAL append has been synced. `--serve` waits for SIGTERM (containers).

## Documentation

| Doc | Purpose |
|-----|---------|
| [docs/roadmap.md](docs/roadmap.md) | Phased plan and acceptance criteria |
| [docs/architecture.md](docs/architecture.md) | Target architecture vs what exists today |
| [docs/storage.md](docs/storage.md) | WAL, MemTable, durability, recovery |
| [docs/consistency.md](docs/consistency.md) | Intended consistency model |
| [docs/benchmarks.md](docs/benchmarks.md) | Measured KV Set/Get |
| [docs/skills.md](docs/skills.md) | How analytics/SQL/Python skills map to this repo |
| [analytics/README.md](analytics/README.md) | ETL + BI export |
| [deploy/k8s/README.md](deploy/k8s/README.md) | StatefulSet |

## Principles

1. **Implement internals.** Storage, recovery, and consensus are first-class code.
2. **Prove, don't advertise.** Throughput and p99 come from benchmarks. Never invent numbers.
3. **Each phase is independently demoable.** A WAL-backed KV store is already interview-grade if you can explain `fsync` and replay.
4. **Correctness before distribution.** A wrong single-node engine plus Raft is still a wrong cluster.

## License

MIT — see [LICENSE](LICENSE).
