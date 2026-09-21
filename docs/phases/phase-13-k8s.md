# Phase 13 — Kubernetes

Run the **same** `nebuladb` binary in a StatefulSet. PersistentVolumeClaim holds the WAL/SST directory. HTTP `/livez` and `/readyz` are for kubelet probes only — not a KV API (ADR-006 still holds; ADR-013 extends HTTP to probes).

This phase does **not** replace Raft with etcd or wrap a cloud database. We do not claim a live cluster until someone has applied these manifests.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 13.0 | This spec | `docs: Phase 13 Kubernetes` |
| 13.1 | `/livez` `/readyz` on the HTTP mux | `feat(http): kube probes` |
| 13.2 | `--serve` + SIGTERM closes the engine | `feat(cmd): container shutdown` |
| 13.3 | Dockerfile + StatefulSet + PVC + headless Service | `feat(deploy): k8s manifests` |

## Definition of done

- [ ] `GET /livez` → 200 while the process is up
- [ ] `GET /readyz` → 200 after open, 503 after shutdown starts
- [ ] `--serve` blocks without a REPL (container stdin is not a TTY)
- [ ] SIGTERM / Interrupt runs `Engine.Close` (WAL fsync) before exit
- [ ] StatefulSet `volumeClaimTemplates` mount `/data`
- [ ] Headless Service (`clusterIP: None`) for stable DNS
- [ ] Rolling restart **intent**: PVC survives pod replacement; committed `Set` with `sync=always` is on disk after Close. We do not invent a measured zero-downtime SLA.

## HTTP (same listener as metrics)

| Path | Use |
|------|-----|
| `/metrics` | Prometheus text (Phase 11) |
| `/livez` | livenessProbe |
| `/readyz` | readinessProbe |

Flags: `--http host:port` (preferred). `--metrics` remains an alias. Empty = no HTTP (REPL-only, as before).

## Container

```
nebuladb --data /data --http :8080 --serve
```

`terminationGracePeriodSeconds` must exceed worst-case Close (WAL fsync). Default 30s.

## What we will not claim

- “Production GKE/EKS operator”
- Helm chart marketplace
- Multi-AZ Raft via Kubernetes until a 3-replica overlay is applied **and** tested on a real cluster
- That these YAML files were executed in CI (unless a later job runs `kind`)
