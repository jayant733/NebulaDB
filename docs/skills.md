# Skills mapped onto NebulaDB

This project is a **from-scratch Go database**, not a Tableau or Snowflake wrapper. Resume skills land as **real work on this engine and a Python analytics layer that uses it**. Do not claim a product we did not implement.

| Skill from the brief | Where it lives | Honest status after Phase 12 |
|----------------------|----------------|------------------------------|
| DBMS fundamentals | WAL, LSM, catalog, txns, Raft | Already in Phases 1–8 |
| Relational design + optimization | Star schema sample; PK + secondary indexes; planner point/index lookup | Schema + indexes exist; cost-based optimizer is later |
| SQL: joins | `INNER JOIN … ON t.a = u.b` | Phase 12 |
| SQL: aggregates | `COUNT` / `SUM` / `AVG` + `GROUP BY` | Phase 12 |
| SQL: views | Saved `.sql` queries in `analytics/sql/` (engine `CREATE VIEW` is later) | Query files, not catalog views |
| SQL: window functions, CTEs, triggers | Specified, not implemented | Later SQL expansion |
| Python gather / clean / normalize | `analytics/etl.py` | Phase 12 |
| ETL | Extract dirty CSV → clean tables → load via Nebula SQL | Phase 12 |
| Tableau / Power BI / Qlik | CSV export plus `analytics/bi/orders.pbids` (Power BI), `analytics/bi/orders_star.tds` (Tableau), HTML Chart.js dashboard. Products are not embedded. | Side feature |
| Spark / Databricks / Snowflake | `spark_job.py` (PySpark if present, else stdlib groupBy); Databricks notebook source; Snowflake `COPY INTO` SQL. Not a live adapter. | Side feature |
| Scheduled pipelines | `analytics/pipeline.py` `--once` or `--interval`; `cron.example`; optional GitHub Action | Side feature |
| Generative AI concepts | Rule-based NL → supported SQL (`analytics/nl_sql.py`); not a trained model | Phase 12 |
| Analytics domain | Retail-style star: `dim_customer`, `fact_order` | Phase 12 |
| Communication | `docs/skills.md` + `analytics/README.md` explain the pipeline | Phase 12 |

## What we will not write on a resume until it exists

- “Built Tableau dashboards inside NebulaDB”
- “Production Snowflake / Databricks / Tableau product” (we ship recipes + local jobs only)
- “Full SQL: windows, CTEs, triggers”
- Invented QPS numbers (Phase 14 benchmarks)
