package storage

import "testing"

func TestInjectDiskErrorsSkipWAL(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	e.InjectDiskErrors(1)
	if err := e.Set([]byte("a"), []byte("1")); err != ErrDisk {
		t.Fatalf("want ErrDisk got %v", err)
	}
	if _, ok, err := e.Get([]byte("a")); err != nil || ok {
		t.Fatalf("key must not exist: %v %v", ok, err)
	}
	if err := e.Set([]byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	_ = e.Close()
	e, err = Open(Options{Dir: dir, Sync: SyncAlways})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	v, ok, err := e.Get([]byte("a"))
	if err != nil || !ok || string(v) != "1" {
		t.Fatalf("recover %q %v %v", v, ok, err)
	}
}
