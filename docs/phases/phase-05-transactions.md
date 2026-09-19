# Phase 5 — Transactions

Single-node SQL transactions. Not distributed 2PC.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 5.0 | Isolation model | `docs: specify transactions and isolation` |
| 5.1 | KV txn: buffer, commit, rollback | `feat(storage): add transactions` |
| 5.2 | SQL BEGIN/COMMIT/ROLLBACK | `feat(sql): transactional DML` |
| 5.3 | REPL | `feat(cmd): BEGIN/COMMIT/ROLLBACK` |
| 5.4 | Docs status | `docs: mark Phase 5 complete` |

## Definition of done

- `BEGIN` / `COMMIT` / `ROLLBACK`
- Uncommitted writes are invisible to other sessions (**no dirty reads**)
- Default isolation: **Read Committed** (a later `SELECT` can see rows committed by others after `BEGIN`)
- Optional **Repeatable Read**: snapshot at `BEGIN`; concurrent commits are not visible; write-write conflict aborts `COMMIT`
- DDL (`CREATE TABLE` / `CREATE INDEX`) is not allowed inside a transaction
- Isolation tests with two sessions on one KV engine

## Client-visible atomicity

`COMMIT` applies the write set under the storage mutex. A process crash **during** commit can leave a prefix of the write set on the WAL (multiple records). That is documented; grouping into one WAL frame is later work.

## SQL

```
BEGIN [TRANSACTION] [READ COMMITTED | REPEATABLE READ] ;
COMMIT [TRANSACTION] ;
ROLLBACK [TRANSACTION] ;
```

Default isolation is Read Committed.
