"""Write a tiny HTML chart dashboard from city_sales.csv (local BI, not Tableau)."""

from __future__ import annotations

import csv
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def render(city_csv: Path, dest: Path) -> None:
    rows = list(csv.DictReader(city_csv.open(newline="", encoding="utf-8")))
    cities = [r["city"] for r in rows]
    revenue = [int(r["revenue"]) for r in rows]
    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <title>NebulaDB city sales</title>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
  <style>
    body {{ font-family: Georgia, serif; max-width: 720px; margin: 2rem auto; color: #222; }}
    canvas {{ max-height: 360px; }}
  </style>
</head>
<body>
  <h1>City revenue</h1>
  <p>Built from <code>city_sales.csv</code> after ETL + Spark job. Open
     <code>analytics/bi/orders.pbids</code> in Power BI, or
     <code>analytics/bi/orders_star.tds</code> in Tableau, or import the CSV in Qlik.</p>
  <canvas id="c"></canvas>
  <script>
    const labels = {json.dumps(cities)};
    const data = {json.dumps(revenue)};
    new Chart(document.getElementById('c'), {{
      type: 'bar',
      data: {{ labels, datasets: [{{ label: 'revenue', data, backgroundColor: '#3d5a80' }}] }},
      options: {{ scales: {{ y: {{ beginAtZero: true }} }} }}
    }});
  </script>
</body>
</html>
"""
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(html, encoding="utf-8")


if __name__ == "__main__":
    import argparse

    ap = argparse.ArgumentParser()
    ap.add_argument("--in", dest="src", type=Path, default=ROOT / "out" / "city_sales.csv")
    ap.add_argument("--out", type=Path, default=ROOT / "out" / "dashboard.html")
    args = ap.parse_args()
    render(args.src, args.out)
    print(args.out)
