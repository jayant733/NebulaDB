package shard

import (
	"testing"

	sqlengine "github.com/jayant/nebuladb/internal/sql/engine"
)

func TestSQLOverRouter(t *testing.T) {
	rt, engs := openRouter(t, 3)
	defer closeEngines(engs)
	e, err := sqlengine.New(rt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Exec(`CREATE TABLE users (id INT, name TEXT, age INT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Exec(`INSERT INTO users VALUES (1, 'Jayant', 20), (2, 'Nebula', 21)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Exec(`CREATE INDEX idx_age ON users (age)`); err != nil {
		t.Fatal(err)
	}
	r, err := e.Exec(`SELECT name FROM users WHERE id = 1`)
	if err != nil || len(r.Rows) != 1 || r.Rows[0][0] != "Jayant" {
		t.Fatalf("pk %+v %v", r, err)
	}
	r, err = e.Exec(`SELECT name FROM users WHERE age = 21`)
	if err != nil || len(r.Rows) != 1 || r.Rows[0][0] != "Nebula" {
		t.Fatalf("idx %+v %v", r, err)
	}
	r, err = e.Exec(`SELECT * FROM users ORDER BY id`)
	if err != nil || len(r.Rows) != 2 {
		t.Fatalf("scan %+v %v", r, err)
	}
}
