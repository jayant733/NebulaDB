# Phase 5 — Transactions

Single-node SQL transactions. Not distributed 2PC.

| Step | Deliverable | Commit theme |
|------|-------------|--------------|
| 5.0 | Isolation model | `docs: specify transactions and isolation` |
| 5.1 | KV txn: buffer, commit, rollback | `feat(storage): add transactions` |
| 5.2 | SQL BEGIN/COMMIT/ROLLBACK | `feat(sql): transactional DML` |
| 5.3 | REPL | `feat(cmd): BEGIN/COMMIT/ROLLBACK` |
| 5.4 | Docs status | this file |

## Definition of done

- [x] `BEGIN` / `COMMIT` / `ROLLBACK`
- [x] Uncommitted writes are invisible to other sessions (**no dirty reads**)
- [x] Default isolation: **Read Committed**
- [x] Optional **Repeatable Read** with write-write abort
- [x] DDL is not allowed inside a transaction
- [x] Isolation tests with two sessions

## Client-visible atomicity

`COMMIT` applies the write set under the storage mutex. A process crash **during** commit can leave a prefix of the write set on the WAL (multiple records). That is documented; grouping into one WAL frame is later work.

## SQL

```
BEGIN [TRANSACTION] [READ COMMITTED | REPEATABLE READ] ;
COMMIT [TRANSACTION] ;
ROLLBACK [TRANSACTION] ;
```

Default isolation is Read Committed.
