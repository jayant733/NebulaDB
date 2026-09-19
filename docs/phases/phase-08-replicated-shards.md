# Phase 8 — Replicated shards

Each **shard** is an independent Raft group. A process hosts one replica of every shard (same `--id` in every group). Transports are **isolated per shard** so elections do not mix.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 8.0 | Group membership and ports | `docs: specify replicated shards` |
| 8.1 | Per-shard listen addresses | `feat(cluster): offset peer ports by shard` |
| 8.2 | Host starts one Raft group per shard | `feat(cluster): Raft group per shard` |
| 8.3 | Leader picker + router tests | `feat(shard): write on the shard leader` |
| 8.4 | `--shards` + `--peers` on `nebuladb` | `feat(cmd): replicated shards` |
| 8.5 | Status docs | `docs: Phase 8 complete` |

## Definition of done

- [ ] Two shards × three nodes: each shard elects its own leader (leaders may differ)
- [ ] A write hashes to a shard and commits on **that** shard's majority
- [ ] Isolating the leader of shard 0 does not block a new write on shard 1
- [ ] After shard 0 failover, a write for a shard-0 key succeeds on the new leader
- [ ] Followers of a shard reject `Propose` (`ErrNotLeader`)
- [ ] `--shards N --peers ...` is allowed; ports are `base+shardIndex`

## Topology

```
Node n1                    Node n2                    Node n3
┌─────────────┐            ┌─────────────┐            ┌─────────────┐
│ shard-0 Raft│◄──────────►│ shard-0 Raft│◄──────────►│ shard-0 Raft│
│ shard-1 Raft│◄──────────►│ shard-1 Raft│◄──────────►│ shard-1 Raft│
└─────────────┘            └─────────────┘            └─────────────┘
```

`--id n1 --shards 2 --peers n1=127.0.0.1:7100,n2=127.0.0.1:7200,n3=127.0.0.1:7300`

| Group | n1 | n2 | n3 |
|-------|----|----|-----|
| shard 0 | :7100 | :7200 | :7300 |
| shard 1 | :7101 | :7201 | :7301 |

Data: `<data>/shard-<i>/` (LSM) and `<data>/shard-<i>/raft/` (Raft log).

## Client writes

A process proposes only if **it** is leader for the owning shard. A SQL `INSERT` of two rows can hit two shards; one `Propose` may return `ErrNotLeader`. Forwarding to the remote leader is Phase 10.

Tests (and a `cluster.LeaderPick` helper) send the write to whichever replica currently leads that shard.

Prefix scans on a replica are local and may lag. Tests read through the shard leader.

## Out of scope

- Cross-shard transactions / 2PC
- Live rebalance while Raft groups are running (Phase 7 rebalance stays local-only)
- gRPC, membership changes, query router as a separate process
