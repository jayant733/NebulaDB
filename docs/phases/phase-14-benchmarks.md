# Phase 14 — Benchmarks + docs

Publish **measured** numbers only. Hardware, git SHA, flags, and method live in [docs/benchmarks.md](../benchmarks.md). Re-run `go run ./cmd/bench` before quoting new figures.

| Step | Deliverable |
|------|-------------|
| 14.0 | This spec |
| 14.1 | `cmd/bench` sequential Set/Get |
| 14.2 | `docs/benchmarks.md` filled from a real run |
| 14.3 | README / architecture / roadmap marked complete for this plan |

## Definition of done

- [x] Bench tool prints `qps`, `p50`, `p99` for Set and Get
- [x] `docs/benchmarks.md` records OS, Go version, SHA, `-n`, sync mode
- [x] No invented throughput in README — link the bench doc
