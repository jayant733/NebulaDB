# Phase 10 — Networking

Client **KV RPC** (`net/rpc` over TCP) so a process that is not the shard leader **forwards** `Set`/`Delete`/`Get` to the current leader. Timeouts, retries on `ErrNotLeader`, and an inflight semaphore provide backpressure.

Raft stays on the existing `net/rpc` transport (Phase 6). A second stack (gRPC) is not added; see ADR-011.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 10.0 | Client protocol | `docs: specify client RPC` |
| 10.1 | KV RPC + timeouts | `feat(netkv): client Get/Set/Delete` |
| 10.2 | Forward to shard leader + retries | `feat(netkv): forward to leader` |
| 10.3 | Inflight limit | `feat(netkv): backpressure` |
| 10.4 | `--rpc` on `nebuladb` | `feat(cmd): client RPC port` |

## Definition of done

- [x] Client `Set` to a follower succeeds after forward to the shard leader
- [x] Retry with leader hint when `ErrNotLeader` races an election
- [x] Call timeout is bounded; inflight RPCs cap concurrent handlers
- [x] `--rpc` listens at `basePort+shardCount` (after per-shard Raft ports)

## Ports

`--id n1 --shards 2 --peers n1=127.0.0.1:7100,...`

| Service | n1 |
|---------|-----|
| Raft shard 0 | :7100 |
| Raft shard 1 | :7101 |
| Client + admin RPC | :7102 |

## Out of scope

gRPC, SQL-over-the-wire, streaming Scan, membership changes, HTTP.
