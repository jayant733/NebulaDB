"""Scheduled analytics pipeline: ETL → Spark job → BI HTML → Snowflake SQL copy.

Default is --once. --interval N repeats (Ctrl+C to stop). This is a process-local
scheduler, not Airflow/Databricks Jobs.
"""

from __future__ import annotations

import argparse
import shutil
import sys
import time
from pathlib import Path

_HERE = Path(__file__).resolve().parent
if str(_HERE) not in sys.path:
    sys.path.insert(0, str(_HERE))

from dashboard import render as render_dash
from etl import RAW, load_raw, to_sql, write_bi_csv
from spark_job import run as spark_run

ROOT = Path(__file__).resolve().parent
OUT = ROOT / "out"
SNOW = ROOT / "snowflake" / "copy_into.sql"


def run_once() -> None:
    dims, facts = load_raw(RAW)
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT / "load.sql").write_text(to_sql(dims, facts), encoding="utf-8")
    write_bi_csv(dims, facts, OUT)
    spark_run(OUT / "orders_star.csv", OUT / "city_sales.csv", prefer_spark=False)
    render_dash(OUT / "city_sales.csv", OUT / "dashboard.html")
    shutil.copyfile(SNOW, OUT / "snowflake_copy_into.sql")
    print(f"pipeline ok customers={len(dims)} orders={len(facts)}")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--once", action="store_true", default=True)
    ap.add_argument("--interval", type=int, default=0, help="seconds between runs; 0 = once")
    ap.add_argument("--max-runs", type=int, default=0, help="stop after N runs when interval > 0")
    args = ap.parse_args()
    n = 0
    while True:
        run_once()
        n += 1
        if args.interval <= 0:
            return
        if args.max_runs and n >= args.max_runs:
            return
        time.sleep(args.interval)


if __name__ == "__main__":
    main()
