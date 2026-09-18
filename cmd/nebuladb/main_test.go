package main

import (
	"path/filepath"
	"testing"

	"github.com/jayant/nebuladb/internal/index"
	"github.com/jayant/nebuladb/internal/storage"
)

func TestREPLSetGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	eng, err := storage.Open(storage.Options{Dir: dir, Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Wrap(eng)
	if err != nil {
		t.Fatal(err)
	}
	r := &repl{eng: eng, idx: idx}
	if err := run(r, "set hello world"); err != nil {
		t.Fatal(err)
	}
	if err := run(r, "get hello"); err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	eng, err = storage.Open(storage.Options{Dir: filepath.Clean(dir), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	v, ok, err := eng.Get([]byte("hello"))
	if err != nil || !ok || string(v) != "world" {
		t.Fatalf("after reopen: %q ok=%v err=%v", v, ok, err)
	}
}

func TestREPLIndexedRow(t *testing.T) {
	eng, err := storage.Open(storage.Options{Dir: t.TempDir(), Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	idx, err := index.Wrap(eng)
	if err != nil {
		t.Fatal(err)
	}
	r := &repl{eng: eng, idx: idx}
	if err := run(r, "idxcreate age 0"); err != nil {
		t.Fatal(err)
	}
	if err := run(r, "row 1 20 jayant"); err != nil {
		t.Fatal(err)
	}
	if err := run(r, "idxfind age 20"); err != nil {
		t.Fatal(err)
	}
	pks, err := idx.Find("age", []byte("20"))
	if err != nil || len(pks) != 1 || string(pks[0]) != "1" {
		t.Fatalf("%q %v", pks, err)
	}
}
