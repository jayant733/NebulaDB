# Phase 9 — Failure injection

Chaos tests that match [docs/consistency.md](../consistency.md). `nebulactl` talks to an **admin RPC** on a live process. In-process tests use the same gates without the CLI.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 9.0 | Fault model | `docs: specify failure injection` |
| 9.1 | Partition gate on Memory + TCP | `feat(raft): partition RPCs` |
| 9.2 | Disk-write injector | `feat(storage): inject disk errors` |
| 9.3 | Majority / minority / disk tests | `test(cluster): chaos partitions` |
| 9.4 | `nebulactl` + admin RPC | `feat(cmd): nebulactl kill-node` |

## Definition of done

- [ ] Majority of a 3-node group still commits after one follower is isolated
- [ ] Leader in a minority cannot commit; the majority elects and a new write succeeds
- [ ] Two partitions do not both accept writes for the same shard (no split-brain commit)
- [ ] Injected disk errors fail `Set` before the WAL record is durable
- [ ] `nebulactl kill-node|partition|heal|disk-fail` call admin RPC

## Faults

| Fault | Mechanism |
|-------|-----------|
| Isolate node | Drop all Raft RPCs to/from that id |
| Symmetric split | Drop RPCs whose endpoints are on different sides |
| Kill | `Node.Stop` (process `os.Exit` from CLI) |
| Disk | Next N `Engine.Set`/`Delete` return `storage.ErrDisk` without appending WAL |

TCP senders consult a per-process `raft.Gate`. Incoming RPCs still arrive unless the peer also drops; isolating a leader is enough for it to lose heartbeats and step down after election timeout.

## Out of scope

Byzantine nodes, clock jumps, bit-rot beyond WAL CRC, chaos on a running Kubernetes cluster (Phase 12).
