package shard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jayant/nebuladb/internal/storage"
)

func TestOpenLocalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rt, engs, err := OpenLocal(dir, 2, storage.SyncNone)
	if err != nil {
		t.Fatal(err)
	}
	defer closeEngines(engs)
	if err := rt.Set([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	v, ok, err := rt.Get([]byte("k"))
	if err != nil || !ok || string(v) != "v" {
		t.Fatalf("%q %v %v", v, ok, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "shard-0")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "shard-1")); err != nil {
		t.Fatal(err)
	}
}
