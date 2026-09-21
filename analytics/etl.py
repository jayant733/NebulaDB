"""Gather, clean, normalize raw orders into a star schema and emit Nebula SQL."""

from __future__ import annotations

import argparse
import csv
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent
RAW = ROOT / "data" / "raw_orders.csv"
OUT = ROOT / "out"


def clean_email(s: str) -> str:
    return s.strip().lower()


def clean_city(s: str) -> str:
    return re.sub(r"\s+", " ", s.strip()).title()


def load_raw(path: Path) -> tuple[list[dict], list[dict]]:
    customers: dict[int, dict] = {}
    orders: list[dict] = []
    with path.open(newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            raw_id = (row.get("id") or "").strip()
            if not raw_id.isdigit():
                continue
            cid = int(raw_id)
            amount_s = (row.get("amount") or "").strip()
            oid_s = (row.get("order_id") or "").strip()
            if not amount_s.isdigit() or not oid_s.isdigit():
                continue
            email = clean_email(row.get("Email") or row.get("email") or "")
            city = clean_city(row.get(" City") or row.get("City") or row.get("city") or "")
            if not email or not city:
                continue
            customers[cid] = {"id": cid, "email": email, "city": city}
            orders.append(
                {
                    "id": int(oid_s),
                    "customer_id": cid,
                    "amount": int(amount_s),
                }
            )
    dims = sorted(customers.values(), key=lambda r: r["id"])
    facts = sorted(orders, key=lambda r: r["id"])
    return dims, facts


def sql_escape(s: str) -> str:
    return s.replace("'", "''")


def to_sql(dims: list[dict], facts: list[dict]) -> str:
    lines = [
        "CREATE TABLE dim_customer (id INT, email TEXT, city TEXT);",
        "CREATE TABLE fact_order (id INT, customer_id INT, amount INT);",
    ]
    for c in dims:
        lines.append(
            f"INSERT INTO dim_customer VALUES ({c['id']}, '{sql_escape(c['email'])}', '{sql_escape(c['city'])}');"
        )
    for o in facts:
        lines.append(
            f"INSERT INTO fact_order VALUES ({o['id']}, {o['customer_id']}, {o['amount']});"
        )
    lines.append(
        "SELECT city, COUNT(*), SUM(amount) FROM fact_order INNER JOIN dim_customer ON fact_order.customer_id = dim_customer.id GROUP BY city;"
    )
    return "\n".join(lines) + "\n"


def write_bi_csv(dims: list[dict], facts: list[dict], dest: Path) -> None:
    dest.mkdir(parents=True, exist_ok=True)
    with (dest / "dim_customer.csv").open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=["id", "email", "city"])
        w.writeheader()
        w.writerows(dims)
    with (dest / "fact_order.csv").open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=["id", "customer_id", "amount"])
        w.writeheader()
        w.writerows(facts)
    # denormalized extract for Tableau / Power BI / Qlik (file import)
    by_id = {c["id"]: c for c in dims}
    with (dest / "orders_star.csv").open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=["order_id", "customer_id", "email", "city", "amount"])
        w.writeheader()
        for o in facts:
            c = by_id[o["customer_id"]]
            w.writerow(
                {
                    "order_id": o["id"],
                    "customer_id": o["customer_id"],
                    "email": c["email"],
                    "city": c["city"],
                    "amount": o["amount"],
                }
            )


def main() -> None:
    ap = argparse.ArgumentParser(description="ETL dirty orders into Nebula SQL + BI CSV")
    ap.add_argument("--raw", type=Path, default=RAW)
    ap.add_argument("--sql-out", type=Path, default=OUT / "load.sql")
    ap.add_argument("--csv-out", type=Path, default=OUT)
    args = ap.parse_args()
    dims, facts = load_raw(args.raw)
    args.sql_out.parent.mkdir(parents=True, exist_ok=True)
    args.sql_out.write_text(to_sql(dims, facts), encoding="utf-8")
    write_bi_csv(dims, facts, args.csv_out)
    print(f"customers={len(dims)} orders={len(facts)}")
    print(f"sql={args.sql_out}")
    print(f"bi csv={args.csv_out / 'orders_star.csv'}")
    print("Load: go run ./cmd/nebuladb --data ./data --no-sync < analytics/out/load.sql")


if __name__ == "__main__":
    main()
