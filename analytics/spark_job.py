"""Spark-shaped aggregation job over the star-schema CSV.

If PySpark is installed, uses SparkSession. Otherwise the same groupBy runs in
the standard library so the job is demoable without a cluster.
This is not a Databricks or EMR deployment.
"""

from __future__ import annotations

import argparse
import csv
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent
STAR = ROOT / "out" / "orders_star.csv"
OUT = ROOT / "out" / "city_sales.csv"


def aggregate_local(star: Path) -> list[dict]:
    totals: dict[str, list[int]] = defaultdict(lambda: [0, 0])  # count, sum
    with star.open(newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            city = row["city"]
            amt = int(row["amount"])
            totals[city][0] += 1
            totals[city][1] += amt
    rows = [
        {"city": city, "orders": n, "revenue": s}
        for city, (n, s) in sorted(totals.items())
    ]
    return rows


def aggregate_spark(star: Path):
    from pyspark.sql import SparkSession
    from pyspark.sql import functions as F

    spark = SparkSession.builder.appName("nebuladb-city-sales").master("local[*]").getOrCreate()
    df = spark.read.option("header", True).csv(str(star))
    out = (
        df.groupBy("city")
        .agg(F.count("*").alias("orders"), F.sum("amount").cast("long").alias("revenue"))
        .orderBy("city")
    )
    rows = [row.asDict() for row in out.collect()]
    spark.stop()
    return [{"city": r["city"], "orders": int(r["orders"]), "revenue": int(r["revenue"])} for r in rows]


def write_csv(rows: list[dict], dest: Path) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    with dest.open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=["city", "orders", "revenue"])
        w.writeheader()
        w.writerows(rows)


def run(star: Path = STAR, dest: Path = OUT, prefer_spark: bool = True) -> tuple[list[dict], str]:
    engine = "local"
    if prefer_spark:
        try:
            rows = aggregate_spark(star)
            engine = "pyspark"
        except Exception:
            rows = aggregate_local(star)
    else:
        rows = aggregate_local(star)
    write_csv(rows, dest)
    return rows, engine


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--star", type=Path, default=STAR)
    ap.add_argument("--out", type=Path, default=OUT)
    ap.add_argument("--local", action="store_true", help="skip PySpark even if installed")
    args = ap.parse_args()
    rows, engine = run(args.star, args.out, prefer_spark=not args.local)
    print(f"engine={engine} rows={len(rows)} out={args.out}")


if __name__ == "__main__":
    main()
