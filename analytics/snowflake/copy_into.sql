-- Snowflake side feature: COPY INTO from the ETL CSVs.
-- Replace ACCOUNT/DB/SCHEMA/STAGE. This does not embed Snowflake in NebulaDB.

CREATE DATABASE IF NOT EXISTS NEBULADB_ANALYTICS;
CREATE SCHEMA IF NOT EXISTS NEBULADB_ANALYTICS.STAR;
USE SCHEMA NEBULADB_ANALYTICS.STAR;

CREATE OR REPLACE FILE FORMAT nebuladb_csv
  TYPE = CSV
  SKIP_HEADER = 1
  FIELD_OPTIONALLY_ENCLOSED_BY = '"';

CREATE OR REPLACE TABLE dim_customer (
  id NUMBER,
  email VARCHAR,
  city VARCHAR
);

CREATE OR REPLACE TABLE fact_order (
  id NUMBER,
  customer_id NUMBER,
  amount NUMBER
);

CREATE OR REPLACE TABLE orders_star (
  order_id NUMBER,
  customer_id NUMBER,
  email VARCHAR,
  city VARCHAR,
  amount NUMBER
);

-- Put files on a stage (example: user stage), then:
-- PUT file://analytics/out/dim_customer.csv @~/nebuladb AUTO_COMPRESS=FALSE;
-- COPY INTO dim_customer FROM @~/nebuladb/dim_customer.csv FILE_FORMAT = (FORMAT_NAME = nebuladb_csv);

COPY INTO dim_customer
  FROM @~/nebuladb/dim_customer.csv
  FILE_FORMAT = (FORMAT_NAME = nebuladb_csv);

COPY INTO fact_order
  FROM @~/nebuladb/fact_order.csv
  FILE_FORMAT = (FORMAT_NAME = nebuladb_csv);

COPY INTO orders_star
  FROM @~/nebuladb/orders_star.csv
  FILE_FORMAT = (FORMAT_NAME = nebuladb_csv);

SELECT city, COUNT(*) AS orders, SUM(amount) AS revenue
FROM orders_star
GROUP BY city
ORDER BY city;
