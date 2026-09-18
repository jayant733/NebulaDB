package index

import (
	"testing"

	"github.com/jayant/nebuladb/internal/storage"
)

func TestIndexFindUpdateDelete(t *testing.T) {
	eng, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	s, err := Wrap(eng)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateIndex("age", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRow([]byte("1"), [][]byte{[]byte("20"), []byte("jayant")}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRow([]byte("2"), [][]byte{[]byte("21"), []byte("nebula")}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRow([]byte("3"), [][]byte{[]byte("20"), []byte("other")}); err != nil {
		t.Fatal(err)
	}
	pks, err := s.Find("age", []byte("20"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pks) != 2 || string(pks[0]) != "1" || string(pks[1]) != "3" {
		t.Fatalf("find 20: %q", pks)
	}
	if err := s.PutRow([]byte("1"), [][]byte{[]byte("22"), []byte("jayant")}); err != nil {
		t.Fatal(err)
	}
	pks, _ = s.Find("age", []byte("20"))
	if len(pks) != 1 || string(pks[0]) != "3" {
		t.Fatalf("after update: %q", pks)
	}
	if err := s.DeleteRow([]byte("3")); err != nil {
		t.Fatal(err)
	}
	pks, _ = s.Find("age", []byte("20"))
	if len(pks) != 0 {
		t.Fatalf("after delete: %q", pks)
	}
}

func TestIndexBackfillAndRecover(t *testing.T) {
	dir := t.TempDir()
	eng, err := storage.Open(storage.Options{Dir: dir, Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Wrap(eng)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.PutRow([]byte("1"), [][]byte{[]byte("a"), []byte("x")})
	_ = s.PutRow([]byte("2"), [][]byte{[]byte("b"), []byte("y")})
	if err := s.CreateIndex("f0", 0); err != nil {
		t.Fatal(err)
	}
	pks, err := s.Find("f0", []byte("b"))
	if err != nil || len(pks) != 1 || string(pks[0]) != "2" {
		t.Fatalf("backfill %q %v", pks, err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	eng, err = storage.Open(storage.Options{Dir: dir, Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	s, err = Wrap(eng)
	if err != nil {
		t.Fatal(err)
	}
	pks, err = s.Find("f0", []byte("a"))
	if err != nil || len(pks) != 1 || string(pks[0]) != "1" {
		t.Fatalf("recover %q %v", pks, err)
	}
}

func TestIndexRange(t *testing.T) {
	eng, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	s, _ := Wrap(eng)
	_ = s.CreateIndex("age", 0)
	_ = s.PutRow([]byte("a"), [][]byte{[]byte("10")})
	_ = s.PutRow([]byte("b"), [][]byte{[]byte("15")})
	_ = s.PutRow([]byte("c"), [][]byte{[]byte("20")})
	pks, err := s.RangeFind("age", []byte("10"), []byte("15"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pks) != 2 || string(pks[0]) != "a" || string(pks[1]) != "b" {
		t.Fatalf("%q", pks)
	}
}
