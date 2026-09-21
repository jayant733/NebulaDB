# Analytics on NebulaDB

Python **gather → clean → normalize → load**, then SQL analytics on the engine.

Tableau / Power BI / Qlik are **not** inside NebulaDB. Spark / Databricks / Snowflake are **not** the database. This folder adds a small side layer: a Spark-shaped job over the exported CSV, a Databricks notebook source, a Snowflake `COPY INTO` template, a local scheduler, and BI connection stubs.

```bash
python analytics/pipeline.py --once
# ETL → city_sales.csv → out/dashboard.html → copies snowflake SQL

go run ./cmd/nebuladb --data ./data --no-sync < analytics/out/load.sql

python analytics/nl_sql.py sales by city
python analytics/spark_job.py --local
python analytics/dashboard.py
```

| Path | What it is |
|------|------------|
| `etl.py` | Dirty CSV → star schema SQL + CSVs |
| `spark_job.py` | `GROUP BY city` on `orders_star.csv` (PySpark optional) |
| `databricks/orders_notebook.py` | Import as Databricks source; reads the same CSV |
| `snowflake/copy_into.sql` | Stage + `COPY INTO` after you PUT the CSVs |
| `pipeline.py` | Run the chain; `--interval 60 --max-runs 3` for a demo loop |
| `cron.example` | cron / Task Scheduler hints |
| `bi/orders.pbids` | Power BI file connection to the CSV |
| `bi/orders_star.tds` | Tableau text-scan datasource |
| `out/dashboard.html` | Local bar chart (Chart.js CDN) |

Star schema: `dim_customer (id, email, city)`, `fact_order (id, customer_id, amount)`.

Engine SQL: `INNER JOIN`, `COUNT` / `SUM` / `AVG`, `GROUP BY`. Not implemented: views, triggers, window functions, CTEs.
