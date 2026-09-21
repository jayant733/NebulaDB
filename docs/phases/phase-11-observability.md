# Phase 11 — Observability

HTTP **`/metrics`** in Prometheus text format (ADR-006: HTTP is allowed for metrics only). No Prometheus/Grafana servers are bundled; a dashboard JSON is checked in for import.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 11.0 | Metric names | `docs: specify /metrics` |
| 11.1 | In-process registry + text exposition | `feat(metrics): Prometheus text format` |
| 11.2 | WAL, compaction, Raft, KV instrumentation | `feat: record fsync, elections, ops` |
| 11.3 | `--metrics` HTTP listener | `feat(cmd): /metrics` |
| 11.4 | Grafana dashboard JSON | `docs: Grafana dashboard` |

## Definition of done

- [x] `GET /metrics` returns Prometheus text (`# TYPE ...`)
- [x] Counters: WAL fsyncs, compactions, Raft elections, KV ops
- [x] Histogram: KV/engine op latency in seconds
- [x] `--metrics host:port` serves the handler (off when flag empty)
- [x] `deploy/grafana/nebuladb.json` graphs those series

## Names

| Metric | Type | When |
|--------|------|------|
| `nebuladb_wal_fsyncs_total` | counter | successful `WAL.Sync` |
| `nebuladb_compactions_total` | counter | SST merge finished |
| `nebuladb_raft_elections_total` | counter | `startElection` |
| `nebuladb_raft_leader_changes_total` | counter | `becomeLeader` |
| `nebuladb_ops_total{op=...}` | counter | get/set/delete/kv |
| `nebuladb_op_duration_seconds{op=...}` | histogram | same ops |

No invented QPS number in the README. QPS is `rate(nebuladb_ops_total[1m])` in Grafana.

## Out of scope

Pushgateway, Alertmanager, tracing, `/debug/pprof` (optional later), live Grafana in-repo.
