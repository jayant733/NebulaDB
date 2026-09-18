package lexer

import "testing"

func TestKeywordsAndOps(t *testing.T) {
	toks, err := Collect(`CREATE TABLE users (id INT, name TEXT); SELECT * FROM users WHERE age >= 18 AND name != 'O''Brien'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Kind{
		KwCreate, KwTable, Ident, LParen, Ident, KwInt, Comma, Ident, KwText, RParen, Semicolon,
		KwSelect, Star, KwFrom, Ident, KwWhere, Ident, Ge, Number, KwAnd, Ident, Ne, String, EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("len %d want %d %+v", len(toks), len(want), toks)
	}
	for i := range want {
		if toks[i].Kind != want[i] {
			t.Fatalf("tok %d: %v want %v (%q)", i, toks[i].Kind, want[i], toks[i].Val)
		}
	}
	if toks[len(toks)-2].Val != "O'Brien" {
		t.Fatalf("string escape %q", toks[len(toks)-2].Val)
	}
}

func TestComment(t *testing.T) {
	toks, err := Collect("-- hi\nSELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if toks[0].Kind != KwSelect || toks[1].Kind != Number || toks[1].Val != "1" {
		t.Fatalf("%+v", toks)
	}
}
