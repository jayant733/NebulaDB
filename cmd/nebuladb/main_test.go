package main

import (
	"path/filepath"
	"testing"

	"github.com/jayant/nebuladb/internal/storage"
)

func TestREPLSetGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	eng, err := storage.Open(storage.Options{Dir: dir, Sync: storage.SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	if err := run(eng, "set hello world"); err != nil {
		t.Fatal(err)
	}
	if err := run(eng, "get hello"); err != nil {
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
