# Phase 4 — SQL Engine

A **focused subset**, not the SQL standard. Aggregates and `JOIN` are out of scope until the subset below is correct.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 4.0 | Grammar, catalog, execution notes | `docs: specify SQL subset` |
| 4.1 | Table-qualified LSM keys | `feat(index): namespace rows by table` |
| 4.2 | Lexer | `feat(sql): lexer` |
| 4.3 | Parser + AST | `feat(sql): parser` |
| 4.4 | Catalog + planner + executor | `feat(sql): execute SQL over LSM` |
| 4.5 | REPL (statements ending in `;`) | this file |

## Definition of done

- [x] Lexer + parser for the listed subset
- [x] `CREATE TABLE` / `CREATE INDEX` catalog
- [x] `INSERT` / `SELECT` / `UPDATE` / `DELETE` with `WHERE`, `ORDER BY`, `LIMIT`
- [x] PK and secondary-index lookup in the planner
- [x] REPL executes SQL

## Grammar

```
CREATE TABLE name ( col INT|TEXT [, col INT|TEXT]* ) ;
CREATE INDEX name ON table ( col ) ;
INSERT INTO name VALUES ( literal [, literal]* ) ;
SELECT *|col [, col]* FROM name
  [WHERE pred]
  [ORDER BY col [ASC|DESC]]
  [LIMIT n] ;
UPDATE name SET col = literal [, col = literal]* [WHERE pred] ;
DELETE FROM name [WHERE pred] ;

pred := col (=|!=|<|>|<=|>=) literal [AND pred]
literal := number | 'string'
```

First column is the **primary key**. Duplicate PK on INSERT is an error.

## Catalog

Persisted in the LSM:

- table name, ordered columns, types
- indexes: name → table + column

Rows stay in `index.Store` with keys namespaced by table.

## Planner (simple)

`WHERE col = literal`:

1. PK column → point `Get`
2. indexed column → `Find`
3. else full scan + filter

Range predicates (`<`, `>`) on an indexed column use `RangeFind` when possible; otherwise scan + filter.

`ORDER BY` / `LIMIT` apply in memory after the row set is collected.

## Types

| SQL | Storage |
|-----|---------|
| `INT` | 8-byte order-preserving signed integer |
| `TEXT` | raw UTF-8 bytes |
