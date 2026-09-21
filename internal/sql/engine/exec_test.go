package engine

import (
	"testing"

	"github.com/jayant/nebuladb/internal/storage"
)

func TestSQLCRUD(t *testing.T) {
	kv, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	e, err := New(kv)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, e, `CREATE TABLE users (id INT, name TEXT, age INT)`)
	mustExec(t, e, `INSERT INTO users VALUES (1, 'Jayant', 20), (2, 'Nebula', 21), (3, 'Ada', 20)`)
	mustExec(t, e, `CREATE INDEX idx_users_age ON users (age)`)

	r, err := e.Exec(`SELECT name FROM users WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 || r.Rows[0][0] != "Jayant" {
		t.Fatalf("pk lookup %+v", r.Rows)
	}

	r, err = e.Exec(`SELECT id FROM users WHERE age = 20 ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 2 || r.Rows[0][0] != "3" || r.Rows[1][0] != "1" {
		t.Fatalf("index eq order %+v", r.Rows)
	}

	r, err = e.Exec(`SELECT name FROM users WHERE age >= 21`)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 || r.Rows[0][0] != "Nebula" {
		t.Fatalf("range %+v", r.Rows)
	}

	mustExec(t, e, `UPDATE users SET age = 22 WHERE id = 1`)
	r, _ = e.Exec(`SELECT age FROM users WHERE id = 1`)
	if r.Rows[0][0] != "22" {
		t.Fatalf("update %v", r.Rows)
	}

	mustExec(t, e, `DELETE FROM users WHERE id = 2`)
	r, _ = e.Exec(`SELECT * FROM users ORDER BY id`)
	if len(r.Rows) != 2 {
		t.Fatalf("after delete %d", len(r.Rows))
	}

	r, err = e.Exec(`SELECT * FROM users ORDER BY age DESC LIMIT 1`)
	if err != nil || len(r.Rows) != 1 || r.Rows[0][2] != "22" {
		t.Fatalf("limit %+v %v", r, err)
	}
}

func TestSQLRecover(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.Open(storage.Options{Dir: dir, Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(kv)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, e, `CREATE TABLE t (id INT, v TEXT)`)
	mustExec(t, e, `INSERT INTO t VALUES (1, 'ok')`)
	_ = kv.Close()

	kv, err = storage.Open(storage.Options{Dir: dir, Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	e, err = New(kv)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Exec(`SELECT v FROM t WHERE id = 1`)
	if err != nil || len(r.Rows) != 1 || r.Rows[0][0] != "ok" {
		t.Fatalf("recover %+v %v", r, err)
	}
}

func TestDuplicatePK(t *testing.T) {
	kv, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	e, _ := New(kv)
	mustExec(t, e, `CREATE TABLE t (id INT, v TEXT)`)
	mustExec(t, e, `INSERT INTO t VALUES (1, 'a')`)
	if _, err := e.Exec(`INSERT INTO t VALUES (1, 'b')`); err == nil {
		t.Fatal("expected duplicate pk")
	}
}

func TestSQLTransactionIsolation(t *testing.T) {
	kv, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	a, err := New(kv)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, a, `CREATE TABLE t (id INT, v TEXT)`)
	mustExec(t, a, `INSERT INTO t VALUES (1, 'base')`)
	b, err := New(kv)
	if err != nil {
		t.Fatal(err)
	}

	mustExec(t, a, `BEGIN`)
	mustExec(t, a, `UPDATE t SET v = 'dirty' WHERE id = 1`)
	r, err := b.Exec(`SELECT v FROM t WHERE id = 1`)
	if err != nil || r.Rows[0][0] != "base" {
		t.Fatalf("dirty read %+v %v", r, err)
	}
	mustExec(t, a, `COMMIT`)
	r, err = b.Exec(`SELECT v FROM t WHERE id = 1`)
	if err != nil || r.Rows[0][0] != "dirty" {
		t.Fatalf("after commit %+v %v", r, err)
	}

	mustExec(t, a, `BEGIN`)
	mustExec(t, a, `INSERT INTO t VALUES (2, 'gone')`)
	mustExec(t, a, `ROLLBACK`)
	r, err = a.Exec(`SELECT * FROM t WHERE id = 2`)
	if err != nil || len(r.Rows) != 0 {
		t.Fatalf("rollback %+v %v", r, err)
	}
}

func TestSQLRepeatableRead(t *testing.T) {
	kv, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	a, _ := New(kv)
	mustExec(t, a, `CREATE TABLE t (id INT, v TEXT)`)
	mustExec(t, a, `INSERT INTO t VALUES (1, 'v1')`)
	b, _ := New(kv)
	mustExec(t, a, `BEGIN REPEATABLE READ`)
	mustExec(t, b, `UPDATE t SET v = 'v2' WHERE id = 1`)
	r, err := a.Exec(`SELECT v FROM t WHERE id = 1`)
	if err != nil || r.Rows[0][0] != "v1" {
		t.Fatalf("RR %+v %v", r, err)
	}
	if _, err := a.Exec(`UPDATE t SET v = 'a' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Exec(`COMMIT`); err == nil {
		t.Fatal("expected write-write conflict")
	}
}

func TestSQLJoinAndGroup(t *testing.T) {
	kv, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	e, err := New(kv)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, e, `CREATE TABLE dim_customer (id INT, email TEXT, city TEXT)`)
	mustExec(t, e, `CREATE TABLE fact_order (id INT, customer_id INT, amount INT)`)
	mustExec(t, e, `INSERT INTO dim_customer VALUES (1, 'a@x.com', 'Pune'), (2, 'b@x.com', 'Pune'), (3, 'c@x.com', 'Delhi')`)
	mustExec(t, e, `INSERT INTO fact_order VALUES (10, 1, 100), (11, 1, 50), (12, 2, 20), (13, 3, 5)`)

	r, err := e.Exec(`SELECT city, COUNT(*), SUM(amount) FROM fact_order INNER JOIN dim_customer ON fact_order.customer_id = dim_customer.id GROUP BY city`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, row := range r.Rows {
		got[row[0]] = row
	}
	if got["Pune"][1] != "3" || got["Pune"][2] != "170" {
		t.Fatalf("pune %+v", r.Rows)
	}
	if got["Delhi"][1] != "1" || got["Delhi"][2] != "5" {
		t.Fatalf("delhi %+v", r.Rows)
	}

	r, err = e.Exec(`SELECT COUNT(*) FROM fact_order`)
	if err != nil || len(r.Rows) != 1 || r.Rows[0][0] != "4" {
		t.Fatalf("count %+v %v", r, err)
	}
}

func mustExec(t *testing.T, e *Engine, q string) {
	t.Helper()
	if _, err := e.Exec(q); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}
