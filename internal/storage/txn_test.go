package storage

import "testing"

func TestTxnNoDirtyRead(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()
	_ = e.Set([]byte("k"), []byte("v1"))

	a := e.Begin(ReadCommitted)
	b := e.Begin(ReadCommitted)
	if err := a.Set([]byte("k"), []byte("v2")); err != nil {
		t.Fatal(err)
	}
	v, ok, err := b.Get([]byte("k"))
	if err != nil || !ok || string(v) != "v1" {
		t.Fatalf("dirty read? %q ok=%v err=%v", v, ok, err)
	}
	if err := a.Commit(); err != nil {
		t.Fatal(err)
	}
	v, ok, err = b.Get([]byte("k"))
	if err != nil || !ok || string(v) != "v2" {
		t.Fatalf("read committed should see commit: %q ok=%v err=%v", v, ok, err)
	}
}

func TestTxnRollback(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()
	a := e.Begin(ReadCommitted)
	_ = a.Set([]byte("k"), []byte("x"))
	if err := a.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.Get([]byte("k")); ok {
		t.Fatal("rollback leaked write")
	}
}

func TestRepeatableReadSnapshot(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()
	_ = e.Set([]byte("k"), []byte("v1"))
	a := e.Begin(RepeatableRead)
	b := e.Begin(ReadCommitted)
	_ = b.Set([]byte("k"), []byte("v2"))
	_ = b.Commit()
	v, ok, err := a.Get([]byte("k"))
	if err != nil || !ok || string(v) != "v1" {
		t.Fatalf("RR saw concurrent commit: %q ok=%v err=%v", v, ok, err)
	}
}

func TestRepeatableReadWriteWriteConflict(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()
	_ = e.Set([]byte("k"), []byte("v1"))
	a := e.Begin(RepeatableRead)
	b := e.Begin(RepeatableRead)
	_ = a.Set([]byte("k"), []byte("a"))
	_ = b.Set([]byte("k"), []byte("b"))
	if err := a.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != ErrConflict {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	v, _, _ := e.Get([]byte("k"))
	if string(v) != "a" {
		t.Fatalf("winner %q", v)
	}
}

func TestTxnOwnWritesVisible(t *testing.T) {
	e := mustOpen(t)
	defer e.Close()
	a := e.Begin(ReadCommitted)
	_ = a.Set([]byte("k"), []byte("mine"))
	v, ok, _ := a.Get([]byte("k"))
	if !ok || string(v) != "mine" {
		t.Fatalf("%q %v", v, ok)
	}
}
