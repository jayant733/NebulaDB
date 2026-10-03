# Benchmarks

These numbers are from **one run** of `go run ./cmd/bench`. They are not a product SLA. Re-run on your machine before quoting them.

## Method

1. Single process, one LSM engine, temp directory.
2. Sequential (not concurrent) `Set` then `Get` of 5000 keys, value 32 bytes, key `k%08d`.
3. Default durability is `SyncAlways` (fsync after every Set). `-no-sync` is MemTable+WAL write without fsync.
4. `qps` and `mean_ns` use wall time for the whole phase (`elapsed / n`). Per-op `p50_ns` is often 0 on Windows because many samples are shorter than the timer resolution; **use mean_ns**.

Command:

```bash
go run ./cmd/bench -n 5000
go run ./cmd/bench -n 5000 -no-sync
```

## Machine

| Field | Value |
|-------|--------|
| OS | Windows 11 Home (10.0.22631) |
| CPU | 12th Gen Intel Core i7-12650H |
| RAM | 16 GiB |
| Go | go1.24.4 windows/amd64 |
| Date | 2026-09-21 |
| Git SHA | recorded at commit time (`git rev-parse HEAD` on this file's commit) |

This run was **not** on Kubernetes. Docker/kind was not used.

## Results (n=5000)

| Op | fsync | qps | mean_ns | p99_ns (sampled) |
|----|-------|-----|---------|------------------|
| set | always | 14749 | 67803 | 999700 |
| get | n/a | 1515106 | 660 | 0 |
| set | none | 330570 | 3025 | 0 |
| get | n/a | 1761121 | 567 | 0 |

## What this does not measure

- Multi-node Raft commit latency
- SQL JOIN / GROUP BY
- Client `net/rpc` path
- Concurrent writers (`go test -race` is correctness, not throughput)
- A cloud VM or PVC
