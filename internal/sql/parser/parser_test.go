package parser

import (
	"testing"

	"github.com/jayant/nebuladb/internal/sql/ast"
)

func TestParseCreateInsertSelect(t *testing.T) {
	s, err := Parse(`CREATE TABLE users (id INT, name TEXT, age INT)`)
	if err != nil {
		t.Fatal(err)
	}
	ct := s.(*ast.CreateTable)
	if ct.Name != "users" || len(ct.Cols) != 3 || ct.Cols[1].Type != ast.TypeText {
		t.Fatalf("%+v", ct)
	}

	s, err = Parse(`INSERT INTO users VALUES (1, 'Jayant', 20), (2, 'Nebula', 21);`)
	if err != nil {
		t.Fatal(err)
	}
	ins := s.(*ast.Insert)
	if ins.Table != "users" || len(ins.Values) != 2 || ins.Values[0][1].Text != "Jayant" {
		t.Fatalf("%+v", ins)
	}

	s, err = Parse(`SELECT name, age FROM users WHERE age >= 18 AND id != 0 ORDER BY age DESC LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	sel := s.(*ast.Select)
	if sel.Star || sel.Table != "users" || sel.OrderCol != "age" || !sel.OrderDesc || sel.Limit != 5 {
		t.Fatalf("%+v", sel)
	}
	if sel.Where == nil || sel.Where.Next == nil {
		t.Fatal("AND predicate")
	}

	s, err = Parse(`CREATE INDEX idx_users_age ON users (age)`)
	if err != nil {
		t.Fatal(err)
	}
	if s.(*ast.CreateIndex).Column != "age" {
		t.Fatal("index col")
	}

	s, err = Parse(`UPDATE users SET age = 21 WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if s.(*ast.Update).Sets[0].Lit.Int != 21 {
		t.Fatal("update")
	}

	s, err = Parse(`DELETE FROM users WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if s.(*ast.Delete).Where.Col != "id" {
		t.Fatal("delete")
	}

	s, err = Parse(`BEGIN REPEATABLE READ`)
	if err != nil {
		t.Fatal(err)
	}
	if s.(*ast.Begin).Iso != ast.IsoRepeatableRead {
		t.Fatal("begin iso")
	}
	if _, err := Parse(`COMMIT`); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(`ROLLBACK TRANSACTION`); err != nil {
		t.Fatal(err)
	}
}

func TestParseSelectStar(t *testing.T) {
	s, err := Parse(`SELECT * FROM users`)
	if err != nil {
		t.Fatal(err)
	}
	if !s.(*ast.Select).Star {
		t.Fatal("star")
	}
}
