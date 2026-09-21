# Databricks notebook source
# MAGIC %md
# MAGIC # NebulaDB → Databricks (side feature)
# MAGIC
# MAGIC Import this file as a Databricks **source** notebook.
# MAGIC It reads the CSV NebulaDB's ETL already wrote. NebulaDB is not Databricks.

# COMMAND ----------

from pyspark.sql import functions as F

path = dbutils.widgets.get("csv_path") if False else "/dbfs/FileStore/nebuladb/orders_star.csv"
df = spark.read.option("header", True).csv(path)
sales = (
    df.groupBy("city")
    .agg(F.count("*").alias("orders"), F.sum("amount").alias("revenue"))
    .orderBy("city")
)
display(sales)

# COMMAND ----------

# MAGIC %sql
# MAGIC -- After CREATE TABLE using the CSV (Databricks SQL warehouse)
# MAGIC -- SELECT city, COUNT(*) AS orders, SUM(amount) AS revenue
# MAGIC -- FROM hive_metastore.default.orders_star
# MAGIC -- GROUP BY city;
