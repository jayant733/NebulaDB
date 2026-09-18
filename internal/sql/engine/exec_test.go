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

func mustExec(t *testing.T, e *Engine, q string) {
	t.Helper()
	if _, err := e.Exec(q); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}
