"""Map a few English questions to the NebulaDB SQL subset (generative-AI *concept*, not a model)."""

from __future__ import annotations

import argparse
import re

PATTERNS: list[tuple[re.Pattern[str], str]] = [
    (
        re.compile(r"sales by city|revenue by city|sum.*city", re.I),
        "SELECT city, COUNT(*), SUM(amount) FROM fact_order INNER JOIN dim_customer ON fact_order.customer_id = dim_customer.id GROUP BY city;",
    ),
    (
        re.compile(r"how many orders|count orders", re.I),
        "SELECT COUNT(*) FROM fact_order;",
    ),
    (
        re.compile(r"list customers|all customers", re.I),
        "SELECT * FROM dim_customer;",
    ),
]


def to_sql(question: str) -> str:
    q = question.strip()
    for pat, sql in PATTERNS:
        if pat.search(q):
            return sql
    raise SystemExit(
        "unsupported question; try: 'sales by city', 'how many orders', 'list customers'"
    )


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("question", nargs="+")
    args = ap.parse_args()
    print(to_sql(" ".join(args.question)))


if __name__ == "__main__":
    main()
