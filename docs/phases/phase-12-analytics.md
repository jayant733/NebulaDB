# Phase 12 — Analytics SQL + Python ETL

Use NebulaDB as the **store** for an analytics pipeline. Engine work: inner join and aggregates. Client work: Python cleansing/ETL and BI-ready export.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 12.0 | Skills map + this spec | `docs: analytics track` |
| 12.1 | `COUNT`/`SUM`/`AVG` + `GROUP BY` | `feat(sql): aggregates` |
| 12.2 | `INNER JOIN … ON` | `feat(sql): inner join` |
| 12.3 | Python ETL + star schema CSV | `feat(analytics): Python ETL` |
| 12.4 | BI export + NL→SQL stub | `feat(analytics): export and NL SQL` |

## Definition of done

- [x] `SELECT COUNT(*), SUM(n) FROM t GROUP BY g`
- [x] `SELECT … FROM a INNER JOIN b ON a.x = b.y`
- [x] Dirty CSV → normalized tables via `analytics/etl.py`
- [x] `analytics/out/*.csv` importable in Tableau / Power BI / Qlik
- [x] Docs state what is **not** implemented (windows, CTEs, triggers, Snowflake)

## SQL additions

```
select_item := * | col | COUNT(*) | COUNT(col) | SUM(col) | AVG(col)
SELECT select_item [, ...] FROM t
  [INNER JOIN u ON t.col = u.col]
  [WHERE pred]
  [GROUP BY col]
  [ORDER BY col [ASC|DESC]]
  [LIMIT n]
```

Join is nested-loop in memory (correct, not a hash/merge join optimizer). `ON` columns may be `table.col`. Unqualified `WHERE`/`GROUP BY` columns must exist on exactly one side.

## Python ETL

1. Read `analytics/data/raw_orders.csv` (mixed case, extra spaces, missing ids).
2. Drop bad rows, trim, lowercase emails, typed INT amounts.
3. Emit `dim_customer` / `fact_order` INSERT SQL.
4. Optional: pipe into `nebuladb` stdin.

## BI

Tableau / Power BI / Qlik connect to **files**, not to Raft. `analytics/export.py` writes TSV/CSV from query results (Go test helper also writes the same shape).

## Side feature (Spark / warehouse / scheduler / BI stubs)

Not the database. Spark (`analytics/spark_job.py`) reads `out/orders_star.csv`. Databricks notebook source and Snowflake `COPY INTO` are recipes. `pipeline.py` is a local interval scheduler. `analytics/bi/` holds Power BI `.pbids` and Tableau `.tds`.

## Later (not this phase)

`CREATE VIEW`, `CREATE TRIGGER`, window functions, `WITH` CTEs, Kubernetes.
